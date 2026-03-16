package store

import (
	"context"
	"fmt"
	"github.com/aspasskiy/gogogot/internal/llm/types"
	"strings"
	"time"
)

// --- Episode ---

type Episode struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	Summary      string    `json:"summary"`
	Tags         []string  `json:"tags"`
	Status       string    `json:"status"` // "active" | "closed"
	RunSummary   string    `json:"run_summary,omitempty"`
	UserMsgCount int       `json:"user_msg_count,omitempty"`
	StartedAt    time.Time `json:"started_at"`
	EndedAt      time.Time `json:"ended_at,omitempty"`
	UpdatedAt    time.Time `json:"updated_at"`

	persister  EpisodePersister `json:"-"`
	messages   []Turn           `json:"-"`
	totalUsage Usage            `json:"-"`
}

func (e *Episode) SetPersister(p EpisodePersister) { e.persister = p }
func (e *Episode) String() string                  { return e.ID }

func (e *Episode) Close() {
	e.Status = "closed"
	e.EndedAt = time.Now()
}

func (e *Episode) Save() error                    { return e.persister.SaveEpisode(e) }
func (e *Episode) LoadMessages() error            { return e.persister.LoadMessages(e) }
func (e *Episode) TextMessages() ([]Message, error) { return e.persister.TextMessages(e) }
func (e *Episode) HasMessages() bool              { return e.persister.HasMessages(e) }

func (e *Episode) AppendMessage(msg Turn) {
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now()
	}
	if msg.Usage != nil {
		e.totalUsage.Add(*msg.Usage)
	}
	e.messages = append(e.messages, msg)
	e.persister.AppendMessage(e, msg)
}

func (e *Episode) ReplaceMessages(msgs []Turn) error {
	e.messages = msgs
	return e.persister.ReplaceMessages(e, msgs)
}

func (e *Episode) Messages() []Turn      { return e.messages }
func (e *Episode) TotalUsage() *Usage    { return &e.totalUsage }
func (e *Episode) SetMessages(msgs []Turn) { e.messages = msgs }

type EpisodeInfo struct {
	ID        string
	Title     string
	Summary   string
	Tags      []string
	Status    string
	StartedAt time.Time
	EndedAt   time.Time
}

// EpisodeSearchFunc is a callback that searches past episodes by query.
type EpisodeSearchFunc func(ctx context.Context, query string) ([]EpisodeInfo, error)

// --- Messages & Usage ---

// Turn is a single message in the LLM conversation context.
// Rich format: includes tool_use, tool_result, images — everything the LLM sees.
type Turn struct {
	Role      string // "user" | "assistant"
	Content   []types.ContentBlock
	Timestamp time.Time
	Usage     *Usage
	Metadata  map[string]any
}

// Message is a text-only representation used for summarization and history display.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Usage tracks token consumption and cost for a run.
type Usage struct {
	InputTokens      int
	OutputTokens     int
	CacheReadTokens  int
	CacheWriteTokens int
	TotalTokens      int
	LLMCalls         int
	ToolCalls        int
	Cost             float64 // estimated USD
	Duration         time.Duration
}

func (u *Usage) Add(other Usage) {
	u.InputTokens += other.InputTokens
	u.OutputTokens += other.OutputTokens
	u.CacheReadTokens += other.CacheReadTokens
	u.CacheWriteTokens += other.CacheWriteTokens
	u.TotalTokens += other.TotalTokens
	u.LLMCalls += other.LLMCalls
	u.ToolCalls += other.ToolCalls
	u.Cost += other.Cost
	u.Duration += other.Duration
}

// --- Memory ---

type MemoryFile struct {
	Name string
	Size int64
}

// --- Skills ---

type Skill struct {
	Name        string
	Description string
	FilePath    string
	Dir         string
}

func FormatSkillsForPrompt(skills []Skill) string {
	if len(skills) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("<available_skills>\n")
	for _, s := range skills {
		fmt.Fprintf(&b, "<skill name=%q description=%q location=%q />\n",
			s.Name, s.Description, s.FilePath)
	}
	b.WriteString("</available_skills>")
	return b.String()
}
