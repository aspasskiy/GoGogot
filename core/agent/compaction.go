package agent

import (
	"context"
	"log/slog"
	"gogogot/infra/llm/types"

	"gogogot/core/agent/orchestration"
	"gogogot/infra/llm"
)

func (a *Agent) maybeCompact(ctx context.Context) error {
	ctxWindow := a.client.ContextWindow()
	if ctxWindow <= 0 {
		return nil
	}

	estimated := orchestration.EstimateTokens(a.session.Messages())
	if !a.config.Compaction.ShouldCompact(estimated, ctxWindow) {
		return nil
	}

	slog.Info("compaction triggered",
		"estimated_tokens", estimated,
		"context_window", ctxWindow,
		"threshold", a.config.Compaction.Threshold,
	)

	err := a.session.Compact(ctx, a.config.Compaction, a.summarize)
	if err != nil {
		return err
	}

	after := orchestration.EstimateTokens(a.session.Messages())
	a.emit(orchestration.EventCompaction, map[string]any{
		"before_tokens": estimated,
		"after_tokens":  after,
	})
	slog.Info("compaction done", "before", estimated, "after", after)
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
	var text string
	for _, block := range resp.Content {
		if block.Type == "text" {
			text += block.Text
		}
	}
	return text, nil
}
