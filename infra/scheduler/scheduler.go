package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/rs/zerolog/log"
)

type TaskExecutor func(ctx context.Context, taskID, command, skill string) (string, error)

var backoffSchedule = []time.Duration{
	30 * time.Second,
	1 * time.Minute,
	5 * time.Minute,
	15 * time.Minute,
	60 * time.Minute,
}

const (
	defaultTaskTimeout = 5 * time.Minute
	maxConcurrentTasks = 2
)

type TaskState struct {
	LastRunAt         time.Time `json:"last_run_at,omitempty"`
	LastStatus        string    `json:"last_status,omitempty"`
	LastError         string    `json:"last_error,omitempty"`
	LastDurationMs    int64     `json:"last_duration_ms,omitempty"`
	ConsecutiveErrors int       `json:"consecutive_errors,omitempty"`
}

type Task struct {
	ID        string    `json:"id"`
	Schedule  string    `json:"schedule"`
	Command   string    `json:"command"`
	Skill     string    `json:"skill,omitempty"`
	Label     string    `json:"label"`
	CreatedAt time.Time `json:"created_at"`
	State     TaskState `json:"state"`

	entryID cron.EntryID
	running atomic.Bool
}

type TaskInfo struct {
	ID        string    `json:"id"`
	Schedule  string    `json:"schedule"`
	Command   string    `json:"command"`
	Skill     string    `json:"skill,omitempty"`
	Label     string    `json:"label"`
	NextRun   time.Time `json:"next_run"`
	CreatedAt time.Time `json:"created_at"`
	State     TaskState `json:"state"`
}

type Scheduler struct {
	mu       sync.Mutex
	cron     *cron.Cron
	tasks    map[string]*Task
	path     string
	executor TaskExecutor
	sem      chan struct{}
}

func New(dataDir string, executor TaskExecutor, loc *time.Location) *Scheduler {
	if loc == nil {
		loc = time.UTC
	}
	return &Scheduler{
		cron:     cron.New(cron.WithLocation(loc)),
		tasks:    make(map[string]*Task),
		path:     filepath.Join(dataDir, "schedules.json"),
		executor: executor,
		sem:      make(chan struct{}, maxConcurrentTasks),
	}
}

func (s *Scheduler) SetExecutor(exec TaskExecutor) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.executor = exec
}

// SetLocation stops the cron, recreates it with a new timezone, and re-adds all tasks.
func (s *Scheduler) SetLocation(loc *time.Location) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ctx := s.cron.Stop()
	<-ctx.Done()

	s.cron = cron.New(cron.WithLocation(loc))
	for _, t := range s.tasks {
		entryID, err := s.cron.AddFunc(t.Schedule, s.makeRunner(t.ID, t.Command, t.Skill))
		if err != nil {
			log.Error().Err(err).Str("id", t.ID).Msg("failed to re-add task after timezone change")
			continue
		}
		t.entryID = entryID
	}
	s.cron.Start()
	log.Info().Str("location", loc.String()).Int("tasks", len(s.tasks)).Msg("scheduler timezone updated")
}

func (s *Scheduler) Start() error {
	if err := s.load(); err != nil && !os.IsNotExist(err) {
		log.Warn().Err(err).Msg("failed to load schedules")
	}
	s.cron.Start()
	log.Info().Int("tasks", len(s.tasks)).Msg("scheduler started")
	return nil
}

func (s *Scheduler) Stop() {
	ctx := s.cron.Stop()
	<-ctx.Done()
}

func (s *Scheduler) Add(id, schedule, command, skill, label string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if existing, ok := s.tasks[id]; ok {
		s.cron.Remove(existing.entryID)
	}

	entryID, err := s.cron.AddFunc(schedule, s.makeRunner(id, command, skill))
	if err != nil {
		return fmt.Errorf("invalid cron expression %q: %w", schedule, err)
	}

	s.tasks[id] = &Task{
		ID:        id,
		Schedule:  schedule,
		Command:   command,
		Skill:     skill,
		Label:     label,
		CreatedAt: time.Now(),
		entryID:   entryID,
	}

	return s.save()
}

func (s *Scheduler) Remove(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, ok := s.tasks[id]
	if !ok {
		return fmt.Errorf("task %q not found", id)
	}

	s.cron.Remove(task.entryID)
	delete(s.tasks, id)
	return s.save()
}

func (s *Scheduler) List() []TaskInfo {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]TaskInfo, 0, len(s.tasks))
	for _, t := range s.tasks {
		info := TaskInfo{
			ID:        t.ID,
			Schedule:  t.Schedule,
			Command:   t.Command,
			Skill:     t.Skill,
			Label:     t.Label,
			CreatedAt: t.CreatedAt,
			State:     t.State,
		}
		if entry := s.cron.Entry(t.entryID); !entry.Next.IsZero() {
			info.NextRun = entry.Next
		}
		out = append(out, info)
	}
	return out
}

func (s *Scheduler) makeRunner(id, command, skill string) func() {
	return func() {
		s.mu.Lock()
		task, ok := s.tasks[id]
		s.mu.Unlock()
		if !ok {
			return
		}

		if !task.running.CompareAndSwap(false, true) {
			log.Warn().Str("id", id).Msg("scheduler: task already running, skipping")
			return
		}
		defer task.running.Store(false)

		if task.State.ConsecutiveErrors > 0 && !task.State.LastRunAt.IsZero() {
			idx := task.State.ConsecutiveErrors - 1
			if idx >= len(backoffSchedule) {
				idx = len(backoffSchedule) - 1
			}
			cooldown := backoffSchedule[idx]
			if time.Since(task.State.LastRunAt) < cooldown {
				log.Info().
					Str("id", id).
					Int("consecutive_errors", task.State.ConsecutiveErrors).
					Dur("cooldown", cooldown).
					Msg("scheduler: backoff active, skipping")
				return
			}
		}

		s.sem <- struct{}{}
		defer func() { <-s.sem }()

		log.Info().Str("id", id).Str("command", command).Msg("scheduler firing task")
		start := time.Now()

		ctx, cancel := context.WithTimeout(context.Background(), defaultTaskTimeout)
		defer cancel()

		output, err := s.executor(ctx, id, command, skill)

		elapsed := time.Since(start)
		state := TaskState{
			LastRunAt:      start,
			LastDurationMs: elapsed.Milliseconds(),
		}

		if err != nil {
			state.LastStatus = "error"
			state.LastError = err.Error()
			state.ConsecutiveErrors = task.State.ConsecutiveErrors + 1
			log.Error().
				Err(err).
				Str("id", id).
				Int("consecutive_errors", state.ConsecutiveErrors).
				Dur("duration", elapsed).
				Msg("scheduled task failed")
		} else {
			state.LastStatus = "ok"
			state.ConsecutiveErrors = 0
			log.Info().
				Str("id", id).
				Int("output_len", len(output)).
				Dur("duration", elapsed).
				Msg("scheduled task completed")
		}

		s.mu.Lock()
		if t, ok := s.tasks[id]; ok {
			t.State = state
			_ = s.save()
		}
		s.mu.Unlock()
	}
}

func (s *Scheduler) load() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return err
	}

	var tasks []*Task
	if err := json.Unmarshal(data, &tasks); err != nil {
		return err
	}

	for _, t := range tasks {
		entryID, err := s.cron.AddFunc(t.Schedule, s.makeRunner(t.ID, t.Command, t.Skill))
		if err != nil {
			log.Error().Err(err).Str("id", t.ID).Str("schedule", t.Schedule).Msg("failed to restore scheduled task")
			continue
		}
		t.entryID = entryID
		s.tasks[t.ID] = t
	}

	log.Info().Int("count", len(s.tasks)).Msg("loaded schedules from disk")
	return nil
}

func (s *Scheduler) save() error {
	tasks := make([]*Task, 0, len(s.tasks))
	for _, t := range s.tasks {
		tasks = append(tasks, t)
	}

	data, err := json.MarshalIndent(tasks, "", "  ")
	if err != nil {
		return err
	}

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o644)
}
