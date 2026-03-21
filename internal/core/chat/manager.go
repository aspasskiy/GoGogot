package chat

import (
	"context"
	"fmt"

	"gogogot/internal/llm"
	"gogogot/internal/tools/store"

	"github.com/rs/zerolog/log"
)

type Manager struct {
	store store.Store
	llm   llm.LLM
	bgGuard
}

func NewManager(st store.Store, client llm.LLM) *Manager {
	return &Manager{
		store:   st,
		llm:     client,
		bgGuard: bgGuard{bgRunning: make(map[string]bool)},
	}
}

// Resolve returns the active chat, creating one if needed, with messages loaded.
func (m *Manager) Resolve() (*store.Chat, error) {
	ch, err := m.loadOrCreateActiveChat()
	if err != nil {
		return nil, err
	}
	if err := ch.LoadMessages(); err != nil {
		return nil, fmt.Errorf("load messages: %w", err)
	}
	return ch, nil
}

// Reset force-closes the current chat and creates a new one (e.g. /new command).
func (m *Manager) Reset(ctx context.Context) error {
	ch, err := m.loadOrCreateActiveChat()
	if err != nil {
		return err
	}

	if ch.HasMessages() {
		if err := m.Close(ctx, ch); err != nil {
			log.Error().Err(err).Msg("chat: failed to close chat on reset")
		}
	}

	_, err = m.createAndMap()
	return err
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
