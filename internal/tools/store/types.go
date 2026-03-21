package store

import (
	"context"
	"fmt"
	"gogogot/internal/llm/types"
	"strings"
	"time"
)

// --- Chat ---

type Chat struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Summary   string    `json:"summary"`
	Tags      []string  `json:"tags"`
	Status    string    `json:"status"` // "active" | "closed"
	UserTurns int       `json:"user_turns"`
	StartedAt time.Time `json:"started_at"`
	EndedAt   time.Time `json:"ended_at,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`

	persister  ChatPersister `json:"-"`
	messages   []Turn        `json:"-"`
	totalUsage Usage         `json:"-"`
}

func (c *Chat) SetPersister(p ChatPersister) { c.persister = p }
func (c *Chat) String() string               { return c.ID }

func (c *Chat) Close() {
	c.Status = "closed"
	c.EndedAt = time.Now()
}

func (c *Chat) Save() error                      { return c.persister.SaveChat(c) }
func (c *Chat) LoadMessages() error              { return c.persister.LoadMessages(c) }
func (c *Chat) TextMessages() ([]Message, error) { return c.persister.TextMessages(c) }
func (c *Chat) HasMessages() bool                { return c.persister.HasMessages(c) }

func (c *Chat) AppendMessage(msg Turn) {
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now()
	}
	if msg.Usage != nil {
		c.totalUsage.Add(*msg.Usage)
	}
	if msg.Role == "user" {
		c.UserTurns++
	}
	c.messages = append(c.messages, msg)
	c.persister.AppendMessage(c, msg)
}

func (c *Chat) ReplaceMessages(msgs []Turn) error {
	c.messages = msgs
	return c.persister.ReplaceMessages(c, msgs)
}

func (c *Chat) Messages() []Turn        { return c.messages }
func (c *Chat) TotalUsage() *Usage      { return &c.totalUsage }
func (c *Chat) SetMessages(msgs []Turn) { c.messages = msgs }

type ChatInfo struct {
	ID        string
	Title     string
	Summary   string
	Tags      []string
	Status    string
	StartedAt time.Time
	EndedAt   time.Time
}

// ChatSearchFunc is a callback that searches past chats by query.
type ChatSearchFunc func(ctx context.Context, query string) ([]ChatInfo, error)

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

// --- Identity ---

type SoulInfo struct {
	Soul string
	User string
}

// --- Memory ---

type MemoryFile struct {
	Name    string
	Size    int64
	Content string
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
