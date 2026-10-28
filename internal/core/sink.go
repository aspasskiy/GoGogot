package core

import (
	"context"

	"github.com/aspasskiy/gogogot/internal/domain"
	"github.com/aspasskiy/gogogot/internal/store"
	"github.com/aspasskiy/gogogot/internal/transport"

	"github.com/rs/zerolog/log"
)

// agentSink is the orchestrator's transport.Sink — the one place that decides
// what an agent event means for the outside world. Persistent events are
// written to the store synchronously (so they are never lost to UI
// backpressure); everything else is forwarded to the UI Bus, lossily, except
// Ask which must be delivered.
type agentSink struct {
	ctx  context.Context
	chat *store.Chat
	bus  *transport.Bus
}

func newAgentSink(ctx context.Context, chat *store.Chat, bus *transport.Bus) *agentSink {
	return &agentSink{ctx: ctx, chat: chat, bus: bus}
}

func (s *agentSink) Emit(ev transport.Event) {
	switch e := ev.(type) {
	case transport.MessageEvent:
		// Append to history: writes the existing messages.jsonl, bumps
		// UserTurns, and folds any per-turn usage — unchanged store behavior.
		s.chat.AppendMessage(e.Turn)
		// Persist chat metadata (UserTurns, UpdatedAt) after user-role turns —
		// the same cadence the agent's conv.Save() used before the refactor
		// (after the user message and after each tool-result turn).
		if e.Turn.Role == string(domain.RoleUser) {
			if err := s.chat.Save(); err != nil {
				log.Error().Err(err).Str("chat", s.chat.ID).Msg("orchestrator: save chat failed")
			}
		}

	case transport.CompactionEvent:
		if err := s.chat.ReplaceMessages(e.Messages); err != nil {
			log.Error().Err(err).Str("chat", s.chat.ID).Msg("orchestrator: compaction rewrite failed")
		}

	case transport.DoneEvent:
		s.chat.TotalUsage().Add(e.Usage)
		s.bus.Emit(ev)

	case transport.AskEvent:
		// Guaranteed delivery: a dropped ask deadlocks the agent on its reply.
		_ = s.bus.EmitBlocking(s.ctx, ev)

	default:
		// UI events (LLMStart/Stream, ToolStart/End, Progress, MidMessage,
		// LoopWarning, Error). Lossy by design — status updates may coalesce.
		s.bus.Emit(ev)
	}
}
