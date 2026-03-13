package episode

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
}

func NewManager(st store.Store, client llm.LLM) *Manager {
	return &Manager{store: st, llm: client}
}

// Resolve returns the active episode. If the user's message starts a new topic,
// the current episode is closed and a fresh one is created.
func (m *Manager) Resolve(ctx context.Context, userMessage string) (*store.Episode, error) {
	ep, err := m.loadOrCreateActiveEpisode()
	if err != nil {
		return nil, err
	}

	if ep.HasMessages() {
		ep.UserMsgCount++

		decision, err := m.classify(ctx, ep, userMessage)
		if err != nil {
			log.Warn().Err(err).Msg("episode: classification failed, continuing current episode")
		} else if decision == decisionNew {
			log.Info().
				Str("old_episode", ep.ID).
				Msg("episode: new topic detected, rotating episode")

			if err := m.Close(ctx, ep); err != nil {
				log.Error().Err(err).Msg("episode: failed to close old episode")
			}

			ep, err = m.createAndMap()
			if err != nil {
				return nil, err
			}
		} else if shouldUpdateRunSummary(ep.UserMsgCount) {
			m.updateRunSummary(ctx, ep)
		}
	}

	if err := ep.LoadMessages(); err != nil {
		return nil, fmt.Errorf("load messages: %w", err)
	}
	return ep, nil
}

// Reset force-closes the current episode and creates a new one (e.g. /new command).
func (m *Manager) Reset(ctx context.Context) error {
	ep, err := m.loadOrCreateActiveEpisode()
	if err != nil {
		return err
	}

	if ep.HasMessages() {
		if err := m.Close(ctx, ep); err != nil {
			log.Error().Err(err).Msg("episode: failed to close episode on reset")
		}
	}

	_, err = m.createAndMap()
	return err
}

func (m *Manager) createAndMap() (*store.Episode, error) {
	ep := m.store.NewEpisode()
	if err := ep.Save(); err != nil {
		return nil, err
	}
	if err := m.store.SetActiveEpisodeID(ep.ID); err != nil {
		return nil, err
	}
	return ep, nil
}

// loadOrCreateActiveEpisode loads the active episode or creates a new one if
// none exists or the stored one is no longer active. This logic was previously
// in the store package but belongs here as episode lifecycle orchestration.
func (m *Manager) loadOrCreateActiveEpisode() (*store.Episode, error) {
	epID, err := m.store.GetActiveEpisodeID()
	if err == nil && epID != "" {
		ep, err := m.store.LoadEpisode(epID)
		if err == nil && ep.Status == "active" {
			return ep, nil
		}
	}
	return m.createAndMap()
}
