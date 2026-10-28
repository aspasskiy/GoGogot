package bridge

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/aspasskiy/gogogot/internal/channel"
	"github.com/aspasskiy/gogogot/internal/channel/render"
	"github.com/aspasskiy/gogogot/internal/transport"

	"github.com/rs/zerolog/log"
)

// readyTimeout is how long Reload waits for a (re)started bridge to report
// ready before returning its status.
const readyTimeout = 10 * time.Second

// Manager runs every channel skill under skillsDir. It is a channel.Channel,
// so the engine sees all skill channels as one; channels can be added,
// changed and restarted while it runs (Reload).
type Manager struct {
	// Rollback, when set, restores a channel skill's stock files (for skills
	// shipped with GoGogot) and returns the files it replaced. A bridge that
	// keeps crashing right after start is rolled back once, so a bad edit
	// does not cut the owner off.
	Rollback func(name string) ([]string, error)

	skillsDir string
	reserved  map[string]bool

	mu      sync.Mutex
	ctx     context.Context // set by Start
	handler channel.Handler
	bridges map[string]*bridge
	idle    map[string]Status // channel skills that are not running, with the reason
}

// NewManager scans skillsDir for channel skills. reserved holds the names of
// built-in channels already running (e.g. "telegram"); a skill channel with
// the same name is not started, so the platform is not polled twice.
func NewManager(skillsDir string, reserved ...string) *Manager {
	m := &Manager{
		skillsDir: skillsDir,
		reserved:  map[string]bool{},
		bridges:   map[string]*bridge{},
		idle:      map[string]Status{},
	}
	for _, r := range reserved {
		m.reserved[r] = true
	}
	entries, _ := os.ReadDir(skillsDir)
	for _, e := range entries {
		if e.IsDir() {
			if _, err := os.Stat(filepath.Join(skillsDir, e.Name(), ManifestFile)); err == nil {
				m.load(e.Name())
			}
		}
	}
	return m
}

// load (re)reads one channel skill and files it as a bridge to run or as
// idle with a reason. Callers hold no lock; it does not start anything.
func (m *Manager) load(name string) *bridge {
	dir := filepath.Join(m.skillsDir, name)
	idle := func(s State, detail string) *bridge {
		log.Info().Str("channel", name).Str("state", string(s)).Str("detail", detail).Msg("channel skill not started")
		m.mu.Lock()
		m.idle[name] = Status{Name: name, State: s, Detail: detail, Since: time.Now()}
		delete(m.bridges, name)
		m.mu.Unlock()
		return nil
	}

	man, err := LoadManifest(dir)
	switch {
	case errors.Is(err, os.ErrNotExist):
		m.mu.Lock()
		delete(m.idle, name)
		delete(m.bridges, name)
		m.mu.Unlock()
		return nil
	case err != nil:
		return idle(StateInvalid, err.Error())
	case !man.enabled():
		return idle(StateDisabled, `"enabled": false in channel.json`)
	case m.reserved[name]:
		return idle(StateReserved, "a built-in channel named "+name+" is running")
	}
	if missing := man.missingEnv(); len(missing) > 0 {
		return idle(StateMissingEnv, "set "+strings.Join(missing, ", "))
	}
	if err := man.checkCommand(dir); err != nil {
		return idle(StateInvalid, err.Error())
	}

	b := newBridge(name, dir, man, m.currentHandler, &surface{m: m, name: name})
	b.rollback = m.rollback
	m.mu.Lock()
	delete(m.idle, name)
	m.bridges[name] = b
	m.mu.Unlock()
	return b
}

func (m *Manager) currentHandler() channel.Handler {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.handler
}

func (m *Manager) rollback(name string) ([]string, error) {
	if m.Rollback == nil {
		return nil, nil
	}
	return m.Rollback(name)
}

// Status returns the named channel skill's status.
func (m *Manager) Status(name string) (Status, bool) {
	m.mu.Lock()
	b := m.bridges[name]
	s, idle := m.idle[name]
	m.mu.Unlock()
	if b != nil {
		return b.status(), true
	}
	return s, idle
}

// Runnable reports whether at least one channel skill will start.
func (m *Manager) Runnable() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.bridges) > 0
}

func (m *Manager) Name() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	names := make([]string, 0, len(m.bridges))
	for n := range m.bridges {
		names = append(names, n)
	}
	sort.Strings(names)
	return strings.Join(names, " and ")
}

// Start launches the bridge processes without waiting for the engine, so
// owner notifications work before Run (e.g. a startup error). Run calls it
// too; it is idempotent.
func (m *Manager) Start(ctx context.Context) {
	m.mu.Lock()
	if m.ctx != nil {
		m.mu.Unlock()
		return
	}
	m.ctx = ctx
	bridges := m.sortedBridges()
	m.mu.Unlock()

	for _, b := range bridges {
		b.start(ctx)
	}
}

func (m *Manager) Run(ctx context.Context, handler channel.Handler) error {
	m.mu.Lock()
	m.handler = handler
	m.mu.Unlock()
	m.Start(ctx)

	<-ctx.Done()

	m.mu.Lock()
	bridges := m.sortedBridges()
	m.mu.Unlock()
	for _, b := range bridges {
		b.stop()
	}
	return ctx.Err()
}

// WaitReady waits until every started bridge reported ready or timeout passed.
func (m *Manager) WaitReady(timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	m.mu.Lock()
	bridges := m.sortedBridges()
	m.mu.Unlock()
	for _, b := range bridges {
		b.waitReady(time.Until(deadline))
	}
}

func (m *Manager) sortedBridges() []*bridge {
	bs := make([]*bridge, 0, len(m.bridges))
	for _, b := range m.bridges {
		bs = append(bs, b)
	}
	sort.Slice(bs, func(i, j int) bool { return bs[i].name < bs[j].name })
	return bs
}

// OwnerReplier sends to the owner through every running channel skill. The
// set is resolved on each call, so channels added later are included.
func (m *Manager) OwnerReplier() transport.Replier {
	return &ownerReplier{m: m}
}

type ownerReplier struct{ m *Manager }

func (o *ownerReplier) current() transport.Replier {
	o.m.mu.Lock()
	bridges := o.m.sortedBridges()
	o.m.mu.Unlock()
	rs := make([]transport.Replier, 0, len(bridges))
	for _, b := range bridges {
		rs = append(rs, render.NewReplier(b.surf, b.chatForOwner()))
	}
	return channel.FanOut(rs...)
}

func (o *ownerReplier) SendText(ctx context.Context, text string) error {
	return o.current().SendText(ctx, text)
}

func (o *ownerReplier) SendFile(ctx context.Context, path, caption string) error {
	return o.current().SendFile(ctx, path, caption)
}

func (o *ownerReplier) SendTyping(ctx context.Context) error {
	return o.current().SendTyping(ctx)
}

func (o *ownerReplier) SendAsk(ctx context.Context, prompt string, kind transport.AskKind, options []transport.AskOption) error {
	return o.current().SendAsk(ctx, prompt, kind, options)
}

func (o *ownerReplier) ConsumeEvents(ctx context.Context, events <-chan transport.Event, replyInbox <-chan string) string {
	return o.current().ConsumeEvents(ctx, events, replyInbox)
}

// List returns the status of every channel skill, running or not.
func (m *Manager) List() []Status {
	m.mu.Lock()
	bridges := m.sortedBridges()
	out := make([]Status, 0, len(bridges)+len(m.idle))
	for _, s := range m.idle {
		out = append(out, s)
	}
	m.mu.Unlock()
	for _, b := range bridges {
		out = append(out, b.status())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Logs returns the last lines the named bridge wrote to stderr.
func (m *Manager) Logs(name string) ([]string, error) {
	m.mu.Lock()
	b := m.bridges[name]
	m.mu.Unlock()
	if b == nil {
		return nil, fmt.Errorf("channel %q is not running", name)
	}
	return b.recentLogs(), nil
}

// Reload stops the named channel if it runs, re-reads its channel.json and
// starts it again when it is runnable — the way to apply edits to a bridge,
// enable a new channel skill or disable one. It waits briefly for the bridge
// to report ready and returns its status.
func (m *Manager) Reload(name string) (Status, error) {
	if name == "" || strings.ContainsAny(name, `/\`) || strings.HasPrefix(name, ".") {
		return Status{}, fmt.Errorf("invalid channel name %q", name)
	}

	m.mu.Lock()
	ctx := m.ctx
	old := m.bridges[name]
	m.mu.Unlock()
	if ctx == nil {
		return Status{}, errors.New("channels are not running yet")
	}
	if old != nil {
		old.stop()
	}

	b := m.load(name)
	if b == nil {
		m.mu.Lock()
		s, ok := m.idle[name]
		m.mu.Unlock()
		if !ok {
			return Status{}, fmt.Errorf("no %s in skill %q", ManifestFile, name)
		}
		return s, nil
	}

	b.start(ctx)
	return b.settle(readyTimeout), nil
}

// WaitSettled waits until the named channel reports ready or crashes, up to
// timeout, and returns its status — to fail startup loudly on a bad setup.
func (m *Manager) WaitSettled(name string, timeout time.Duration) (Status, bool) {
	m.mu.Lock()
	b := m.bridges[name]
	m.mu.Unlock()
	if b == nil {
		return m.Status(name)
	}
	return b.settle(timeout), true
}

// IsChannel reports whether the named skill is a channel (has channel.json).
func (m *Manager) IsChannel(name string) bool {
	_, err := os.Stat(filepath.Join(m.skillsDir, name, ManifestFile))
	return err == nil
}

// surface is the render.Surface repliers hold for a channel skill. It looks
// the bridge up by name on every call, and while the channel is restarting
// (no process yet) it waits up to readyTimeout instead of failing — so the
// run that called channel_restart can still deliver its answer.
type surface struct {
	m    *Manager
	name string
}

func (s *surface) bridge(ctx context.Context) (*bridge, error) {
	deadline := time.Now().Add(readyTimeout)
	for {
		s.m.mu.Lock()
		b := s.m.bridges[s.name]
		_, idle := s.m.idle[s.name]
		s.m.mu.Unlock()

		switch {
		case b != nil && b.running():
			return b, nil
		case b == nil && idle:
			return nil, fmt.Errorf("channel %s is not running", s.name)
		case b != nil && b.status().State == StateCrashed:
			return b, nil // let the call fail with the bridge's own error
		case time.Now().After(deadline):
			if b != nil {
				return b, nil
			}
			return nil, fmt.Errorf("channel %s is not running", s.name)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func (s *surface) Caps() render.Caps {
	s.m.mu.Lock()
	b := s.m.bridges[s.name]
	s.m.mu.Unlock()
	if b == nil {
		return render.Caps{Format: render.FormatMarkdown}
	}
	return b.Caps()
}

func (s *surface) Send(ctx context.Context, chat string, t render.Text) (string, error) {
	b, err := s.bridge(ctx)
	if err != nil {
		return "", err
	}
	return b.Send(ctx, chat, t)
}

func (s *surface) Edit(ctx context.Context, chat, msgID string, t render.Text) error {
	b, err := s.bridge(ctx)
	if err != nil {
		return err
	}
	return b.Edit(ctx, chat, msgID, t)
}

func (s *surface) Delete(ctx context.Context, chat, msgID string) error {
	b, err := s.bridge(ctx)
	if err != nil {
		return err
	}
	return b.Delete(ctx, chat, msgID)
}

func (s *surface) Typing(ctx context.Context, chat string) error {
	b, err := s.bridge(ctx)
	if err != nil {
		return err
	}
	return b.Typing(ctx, chat)
}

func (s *surface) SendFile(ctx context.Context, chat, path, caption string) error {
	b, err := s.bridge(ctx)
	if err != nil {
		return err
	}
	return b.SendFile(ctx, chat, path, caption)
}

func (s *surface) Ask(ctx context.Context, chat string, t render.Text, options []transport.AskOption) error {
	b, err := s.bridge(ctx)
	if err != nil {
		return err
	}
	return b.Ask(ctx, chat, t, options)
}
