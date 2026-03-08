package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gogogot/llm"
	"gogogot/llm/types"

	"github.com/rs/zerolog/log"
)

type CompactionConfig struct {
	Threshold      float64 // 0.0–1.0, fraction of context window that triggers compaction
	SafetyMargin   float64 // 1.2 = 20% buffer for token estimate inaccuracy
	PreserveRecent int     // number of recent messages to keep uncompressed
	SummaryPrompt  string  // instruction for the summarization LLM call
}

func DefaultCompactionConfig() CompactionConfig {
	return CompactionConfig{
		Threshold:      0.8,
		SafetyMargin:   1.2,
		PreserveRecent: 5,
		SummaryPrompt:  "Summarize the conversation so far. Preserve decisions, TODOs, constraints, errors, file paths mentioned, the current plan, and task_plan checklist state (task IDs, titles, statuses).",
	}
}

// ShouldCompact returns true when the estimated token count (with safety
// margin) exceeds the compaction threshold of the context window.
func (cc *CompactionConfig) ShouldCompact(estimatedTokens, contextWindow int) bool {
	if contextWindow <= 0 || cc.Threshold <= 0 {
		return false
	}
	adjusted := float64(estimatedTokens) * cc.SafetyMargin
	limit := cc.Threshold * float64(contextWindow)
	return adjusted > limit
}

// Summarizer is a function that takes a conversation transcript and returns
// a concise summary. The agent layer provides this by wrapping an LLM call
// with NoTools=true.
type Summarizer func(ctx context.Context, prompt string) (string, error)

// Compact summarizes old messages and replaces them with a single summary,
// keeping the most recent PreserveRecent messages intact.
func (s *Session) Compact(ctx context.Context, cfg CompactionConfig, summarize Summarizer) error {
	n := len(s.messages)
	if n <= cfg.PreserveRecent {
		return nil
	}

	cutoff := n - cfg.PreserveRecent
	old := s.messages[:cutoff]
	recent := s.messages[cutoff:]

	transcript := renderTranscript(old)
	prompt := cfg.SummaryPrompt + "\n\n---\n\n" + transcript

	summary, err := summarize(ctx, prompt)
	if err != nil {
		return fmt.Errorf("compaction summarize: %w", err)
	}

	compacted := make([]Message, 0, 1+len(recent))
	compacted = append(compacted, Message{
		Role:      string(types.RoleUser),
		Content:   []types.ContentBlock{types.TextBlock("[Context Summary]\n" + summary)},
		Timestamp: time.Now(),
		Metadata:  map[string]any{"compacted": true, "original_messages": len(old)},
	})
	compacted = append(compacted, recent...)
	s.messages = compacted

	return nil
}

func (a *Agent) maybeCompact(ctx context.Context) error {
	ctxWindow := a.client.ContextWindow()
	if ctxWindow <= 0 {
		return nil
	}

	estimated := EstimateTokens(a.session.Messages())
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

	after := EstimateTokens(a.session.Messages())
	a.emit(EventCompaction, CompactionData{
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

func renderTranscript(msgs []Message) string {
	var sb strings.Builder
	for _, m := range msgs {
		fmt.Fprintf(&sb, "[%s]: ", m.Role)
		sb.WriteString(contentToString(m.Content))
		sb.WriteByte('\n')
	}
	return sb.String()
}

func contentToString(blocks []types.ContentBlock) string {
	var sb strings.Builder
	for _, b := range blocks {
		switch b.Type {
		case "text":
			sb.WriteString(b.Text)
		case "tool_use":
			fmt.Fprintf(&sb, "[tool_use: %s]", b.ToolName)
		case "tool_result":
			sb.WriteString(b.ToolOutput)
		case "image":
			sb.WriteString("[image]")
		}
	}
	return sb.String()
}
