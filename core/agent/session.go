package agent

import (
	"gogogot/infra/llm/types"
	"time"
)

type Message struct {
	Role      string // "user" | "assistant"
	Content   []types.ContentBlock
	Timestamp time.Time
	Usage     *Usage
	Metadata  map[string]any
}

type Session struct {
	ID         string
	FilePath   string
	messages   []Message
	TotalUsage Usage
}

func NewSession(id, filePath string) *Session {
	return &Session{
		ID:       id,
		FilePath: filePath,
		messages: make([]Message, 0),
	}
}

func (s *Session) Append(msg Message) {
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now()
	}
	if msg.Usage != nil {
		s.TotalUsage.Add(*msg.Usage)
	}
	s.messages = append(s.messages, msg)
}

func (s *Session) Messages() []Message {
	return s.messages
}

func (s *Session) CompactAll(reason string) {
	s.messages = []Message{
		{
			Role:      string(types.RoleAssistant),
			Content:   []types.ContentBlock{types.TextBlock("Context compacted. Reason: " + reason)},
			Timestamp: time.Now(),
		},
	}
}
