package store

import (
	"time"

	"github.com/aspasskiy/gogogot/internal/domain"
)

// --- Chat (active record: carries a back-reference to its persister) ---

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
	messages   []domain.Turn `json:"-"`
	totalUsage domain.Usage  `json:"-"`
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

func (c *Chat) AppendMessage(msg domain.Turn) {
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

func (c *Chat) ReplaceMessages(msgs []domain.Turn) error {
	c.messages = msgs
	return c.persister.ReplaceMessages(c, msgs)
}

func (c *Chat) Messages() []domain.Turn        { return c.messages }
func (c *Chat) TotalUsage() *domain.Usage      { return &c.totalUsage }
func (c *Chat) SetMessages(msgs []domain.Turn) { c.messages = msgs }

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
