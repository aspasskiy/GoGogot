package chat

import (
	"context"
	"fmt"
	"sync"

	"github.com/aspasskiy/gogogot/internal/llm"
	"github.com/aspasskiy/gogogot/internal/store"

	"github.com/rs/zerolog/log"
)

type Manager struct {
	store store.Store
	llm   llm.LLM

	// active is the cached active chat with its messages loaded. Every run and
	// every background summarization share this one object, so their updates
	// never overwrite each other with a stale copy.
	mu     sync.Mutex
	active *store.Chat

	// bgRunning tracks chats with an in-flight background summarization.
	bgMu      sync.Mutex
	bgRunning map[string]bool
}

func NewManager(st store.Store, client llm.LLM) *Manager {
	return &Manager{
		store:     st,
		llm:       client,
		bgRunning: make(map[string]bool),
	}
}

// Resolve returns the active chat, creating one if needed, with messages loaded.
func (m *Manager) Resolve() (*store.Chat, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.active != nil {
		return m.active, nil
	}

	ch, err := m.loadOrCreateActiveChat()
	if err != nil {
		return nil, err
	}
	if err := ch.LoadMessages(); err != nil {
		return nil, fmt.Errorf("load messages: %w", err)
	}
	m.active = ch
	return ch, nil
}

// Reset force-closes the current chat and creates a new one (e.g. /new command).
func (m *Manager) Reset(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	ch := m.active
	if ch == nil {
		var err error
		if ch, err = m.loadOrCreateActiveChat(); err != nil {
			return err
		}
	}

	if ch.HasMessages() {
		if err := m.Close(ctx, ch); err != nil {
			log.Error().Err(err).Msg("chat: failed to close chat on reset")
		}
	}

	next, err := m.createAndMap()
	if err != nil {
		m.active = nil
		return err
	}
	m.active = next
	return nil
}

func (m *Manager) createAndMap() (*store.Chat, error) {
	ch := m.store.NewChat()
	if err := ch.Save(); err != nil {
		return nil, err
	}
	if err := m.store.SetActiveChatID(ch.ID); err != nil {
		return nil, err
	}
	return ch, nil
}

func (m *Manager) loadOrCreateActiveChat() (*store.Chat, error) {
	chatID, err := m.store.GetActiveChatID()
	if err == nil && chatID != "" {
		ch, err := m.store.LoadChat(chatID)
		if err == nil && ch.Status == "active" {
			return ch, nil
		}
	}
	return m.createAndMap()
}
