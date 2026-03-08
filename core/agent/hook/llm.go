package hook

import (
	"context"
	"fmt"
	"strings"

	"gogogot/llm/types"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func LLMLoggingBeforeHook() BeforeLLMCallFunc {
	return func(_ context.Context, lc *LLMCallContext) {
		evt := log.Debug().
			Str("model", lc.Model).
			Int("message_count", len(lc.Messages))

		if lc.System != "" {
			evt.Str("system_prompt", truncateStr(lc.System, 300))
		}

		arr := zerolog.Arr()
		for _, msg := range lc.Messages {
			arr.Dict(zerolog.Dict().
				Str("role", string(msg.Role)).
				Str("types", blockTypeSummary(msg.Content)).
				Str("preview", truncateStr(textFromBlocks(msg.Content), 150)))
		}
		evt.Array("messages", arr)

		evt.Msg("llm call start")
	}
}

func LLMLoggingAfterHook() AfterLLMCallFunc {
	return func(_ context.Context, lc *LLMCallContext, result *LLMCallResult) {
		resp := result.Response
		evt := log.Info().
			Str("model", lc.Model).
			Dur("elapsed", result.Duration).
			Int("input_tokens", resp.InputTokens).
			Int("output_tokens", resp.OutputTokens).
			Str("stop_reason", resp.StopReason)

		text := textFromBlocks(resp.Content)
		if text != "" {
			evt.Str("response_text", text)
		}

		for _, b := range resp.Content {
			if b.Type == "tool_use" {
				evt.Str("tool_call_"+b.ToolName, string(b.ToolInput))
			}
		}

		evt.Msg("llm call done")
	}
}

func CalcCost(inputPerM, outputPerM float64, inputTokens, outputTokens int) float64 {
	return float64(inputTokens)/1_000_000*inputPerM +
		float64(outputTokens)/1_000_000*outputPerM
}

func textFromBlocks(blocks []types.ContentBlock) string {
	var sb strings.Builder
	for _, b := range blocks {
		if b.Type == "text" && b.Text != "" {
			if sb.Len() > 0 {
				sb.WriteByte(' ')
			}
			sb.WriteString(b.Text)
		}
	}
	return sb.String()
}

func blockTypeSummary(blocks []types.ContentBlock) string {
	seen := make(map[string]int)
	for _, b := range blocks {
		seen[b.Type]++
	}
	var parts []string
	for t, n := range seen {
		if n == 1 {
			parts = append(parts, t)
		} else {
			parts = append(parts, fmt.Sprintf("%s×%d", t, n))
		}
	}
	return strings.Join(parts, ",")
}

func truncateStr(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
