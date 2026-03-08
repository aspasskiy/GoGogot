package agent

import (
	"context"

	"gogogot/core/agent/event"
	"gogogot/core/agent/session"
	"gogogot/llm"
	"gogogot/llm/types"

	"github.com/rs/zerolog/log"
)

func (a *Agent) maybeCompact(ctx context.Context) error {
	ctxWindow := a.client.ContextWindow()
	if ctxWindow <= 0 {
		return nil
	}

	estimated := session.EstimateTokens(a.session.Messages())
	if !a.config.Compaction.ShouldCompact(estimated, ctxWindow) {
		return nil
	}

	log.Info().
		Int("estimated_tokens", estimated).
		Int("context_window", ctxWindow).
		Float64("threshold", a.config.Compaction.Threshold).
		Msg("compaction triggered")

	err := a.session.Compact(ctx, a.config.Compaction, a.summarize)
	if err != nil {
		return err
	}

	after := session.EstimateTokens(a.session.Messages())
	a.emit(event.Compaction, event.CompactionData{
		BeforeTokens: estimated,
		AfterTokens:  after,
	})
	log.Info().Int("before", estimated).Int("after", after).Msg("compaction done")
	return nil
}

func (a *Agent) summarize(ctx context.Context, prompt string) (string, error) {
	msgs := []types.Message{
		types.NewUserMessage(types.TextBlock(prompt)),
	}
	resp, err := a.client.Call(ctx, msgs, llm.CallOptions{
		System:  a.config.Compaction.SummaryPrompt,
		NoTools: true,
	})
	if err != nil {
		return "", err
	}
	return types.ExtractText(resp.Content), nil
}
