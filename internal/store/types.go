package store

import (
	"slices"
	"sync"
	"time"

	"github.com/aspasskiy/gogogot/internal/domain"
)

// --- Chat (active record: carries a back-reference to its persister) ---

// Chat is shared between the agent run (which appends turns) and background
// summarization (which sets title/summary), so its state is guarded by mu.
// Mutate metadata through Update; read it through Info / UserTurnCount.
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

	mu         sync.Mutex
	persister  ChatPersister
	messages   []domain.Turn
	totalUsage domain.Usage
}

func (c *Chat) SetPersister(p ChatPersister) { c.persister = p }
func (c *Chat) String() string               { return c.ID }

// Update applies fn to the chat metadata under the chat lock and persists the
// result. fn may be nil to just save.
func (c *Chat) Update(fn func(c *Chat)) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if fn != nil {
		fn(c)
	}
	return c.persister.SaveChat(c)
}

func (c *Chat) Save() error { return c.Update(nil) }

// Info returns a consistent snapshot of the chat metadata.
func (c *Chat) Info() domain.ChatInfo {
	c.mu.Lock()
	defer c.mu.Unlock()
	return domain.ChatInfo{
		ID:        c.ID,
		Title:     c.Title,
		Summary:   c.Summary,
		Tags:      c.Tags,
		Status:    c.Status,
		StartedAt: c.StartedAt,
		EndedAt:   c.EndedAt,
	}
}

func (c *Chat) UserTurnCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.UserTurns
}

func (c *Chat) LoadMessages() error              { return c.persister.LoadMessages(c) }
func (c *Chat) TextMessages() ([]Message, error) { return c.persister.TextMessages(c) }
func (c *Chat) HasMessages() bool                { return c.persister.HasMessages(c) }

// AppendMessage adds a turn to the in-memory history and appends it to disk.
// Only turns the user actually sent count towards UserTurns — tool-result
// turns share the user role but are produced by the agent.
func (c *Chat) AppendMessage(msg domain.Turn) error {
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now()
	}
	c.mu.Lock()
	if msg.Usage != nil {
		c.totalUsage.Add(*msg.Usage)
	}
	if msg.Role == string(domain.RoleUser) && !domain.IsToolResultTurn(msg.Content) {
		c.UserTurns++
	}
	c.messages = append(c.messages, msg)
	c.mu.Unlock()
	return c.persister.AppendMessage(c, msg)
}

func (c *Chat) ReplaceMessages(msgs []domain.Turn) error {
	c.SetMessages(msgs)
	return c.persister.ReplaceMessages(c, msgs)
}

// Messages returns a copy of the in-memory history.
func (c *Chat) Messages() []domain.Turn {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.messages)
}

func (c *Chat) SetMessages(msgs []domain.Turn) {
	c.mu.Lock()
	c.messages = msgs
	c.mu.Unlock()
}

func (c *Chat) AddUsage(u domain.Usage) {
	c.mu.Lock()
	c.totalUsage.Add(u)
	c.mu.Unlock()
}

// --- Store-local projections (persistence concerns, not domain) ---

// Message is a text-only representation used for summarization and history display.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type SoulInfo struct {
	Soul string
	User string
}

type MemoryFile struct {
	Name    string
	Size    int64
	Content string
}
