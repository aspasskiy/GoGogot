package hook

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aspasskiy/gogogot/internal/domain"
	"github.com/aspasskiy/gogogot/internal/llm"

	"github.com/rs/zerolog/log"
)

const charsPerToken = 4

// EstimateTokens approximates token count from messages (chars / 4).
func EstimateTokens(messages []domain.Turn) int {
	var chars int
	for _, m := range messages {
		chars += estimateBlocksChars(m.Content)
	}
	return chars / charsPerToken
}

func estimateBlocksChars(blocks []domain.ContentBlock) int {
	var n int
	for _, b := range blocks {
		switch b.Type {
		case domain.BlockText:
			n += len(b.Text)
		case domain.BlockToolUse:
			n += len(b.ToolName) + len(b.ToolInput)
		case domain.BlockToolResult:
			n += len(b.ToolOutput)
		case domain.BlockImage:
			n += 1000
		case domain.BlockThinking, domain.BlockRedactedThinking:
			n += len(b.Text) + len(b.Signature)
		}
	}
	return n
}

func renderTranscript(msgs []domain.Turn) string {
	var sb strings.Builder
	for _, m := range msgs {
		fmt.Fprintf(&sb, "[%s]: ", m.Role)
		sb.WriteString(contentToString(m.Content))
		sb.WriteByte('\n')
	}
	return sb.String()
}

func contentToString(blocks []domain.ContentBlock) string {
	var sb strings.Builder
	for _, b := range blocks {
		switch b.Type {
		case domain.BlockText:
			sb.WriteString(b.Text)
		case domain.BlockToolUse:
			fmt.Fprintf(&sb, "[tool_use: %s]", b.ToolName)
		case domain.BlockToolResult:
			sb.WriteString(b.ToolOutput)
		case domain.BlockImage:
			sb.WriteString("[image]")
		}
	}
	return sb.String()
}

// Compaction is a self-contained compaction hook. Create with NewCompaction
// and register via BeforeHook(). Uses LLM from IterationContext for summarization.
type Compaction struct {
	Threshold      float64 // 0.0–1.0, fraction of context window that triggers compaction
	SafetyMargin   float64 // 1.2 = 20 % buffer for token estimate inaccuracy
	PreserveRecent int     // number of recent messages to keep uncompressed
	SummaryPrompt  string  // instruction for the summarization LLM call
}

func NewCompaction() *Compaction {
	return &Compaction{
		Threshold:      0.8,
		SafetyMargin:   1.2,
		PreserveRecent: 5,
		SummaryPrompt:  "Summarize the conversation so far. Preserve decisions, TODOs, constraints, errors, file paths mentioned, the current plan, and task_plan checklist state (task IDs, titles, statuses).",
	}
}

func (c *Compaction) shouldCompact(estimatedTokens, contextWindow int) bool {
	if contextWindow <= 0 || c.Threshold <= 0 {
		return false
	}
	adjusted := float64(estimatedTokens) * c.SafetyMargin
	limit := c.Threshold * float64(contextWindow)
	return adjusted > limit
}

// BeforeHook returns a BeforeIterationFunc that compacts the working set of
// messages when they exceed the context window threshold. It rewrites
// ic.Messages in place and sets ic.Compacted; the agent emits the resulting
// CompactionEvent and the orchestrator persists the new history.
func (c *Compaction) BeforeHook() BeforeIterationFunc {
	return func(ctx context.Context, ic *IterationContext) {
		if ic.ContextWindow <= 0 || ic.LLM == nil {
			return
		}

		msgs := ic.Messages
		estimated := EstimateTokens(msgs)
		if !c.shouldCompact(estimated, ic.ContextWindow) {
			return
		}

		log.Info().
			Int("estimated_tokens", estimated).
			Int("context_window", ic.ContextWindow).
			Float64("threshold", c.Threshold).
			Msg("compaction triggered")

		n := len(msgs)
		if n <= c.PreserveRecent {
			return
		}

		cutoff := n - c.PreserveRecent
		old := msgs[:cutoff]
		recent := msgs[cutoff:]

		transcript := renderTranscript(old)
		prompt := c.SummaryPrompt + "\n\n---\n\n" + transcript

		resp, err := ic.LLM.Call(ctx, []domain.Message{domain.NewUserMessage(domain.TextBlock(prompt))}, llm.CallOptions{
			System:  c.SummaryPrompt,
			NoTools: true,
		})
		if err != nil {
			log.Error().Err(err).Msg("compaction summarize failed")
			return
		}
		summary := domain.ExtractText(resp.Content)

		compacted := make([]domain.Turn, 0, 1+len(recent))
		compacted = append(compacted, domain.Turn{
			Role:      string(domain.RoleUser),
			Content:   []domain.ContentBlock{domain.TextBlock(fmt.Sprintf("[Context Summary]\n%s", summary))},
			Timestamp: time.Now(),
			Metadata:  map[string]any{"compacted": true, "original_messages": len(old)},
		})
		compacted = append(compacted, recent...)

		ic.Messages = compacted
		ic.Compacted = true

		after := EstimateTokens(compacted)
		log.Info().Int("before", estimated).Int("after", after).Msg("compaction done")
	}
}
