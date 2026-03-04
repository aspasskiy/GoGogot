package llm

import (
	"fmt"
	"strings"
	"time"

	"gogogot/infra/llm/types"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func logCallStart(model string, system string, messages []Message, tools []ToolDef, backend string) {
	evt := log.Debug().
		Str("model", model).
		Str("backend", backend).
		Int("message_count", len(messages)).
		Int("tool_count", len(tools))

	if system != "" {
		evt.Str("system_prompt", truncate(system, 300))
	}

	arr := zerolog.Arr()
	for _, msg := range messages {
		arr.Dict(zerolog.Dict().
			Str("role", string(msg.Role)).
			Str("types", blockTypes(msg.Content)).
			Str("preview", truncate(textContent(msg.Content), 150)))
	}
	evt.Array("messages", arr)

	evt.Msg("llm call start")
}

func logCallDone(model string, resp *types.Response, elapsed time.Duration) {
	evt := log.Info().
		Str("model", model).
		Dur("elapsed", elapsed).
		Int("input_tokens", resp.InputTokens).
		Int("output_tokens", resp.OutputTokens).
		Str("stop_reason", resp.StopReason)

	preview := truncate(textContent(resp.Content), 150)
	if preview != "" {
		evt.Str("response_preview", preview)
	}

	var toolNames []string
	for _, b := range resp.Content {
		if b.Type == "tool_use" {
			toolNames = append(toolNames, b.ToolName)
		}
	}
	if len(toolNames) > 0 {
		evt.Strs("tool_calls", toolNames)
	}

	evt.Msg("llm call done")
}

func textContent(blocks []ContentBlock) string {
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

func blockTypes(blocks []ContentBlock) string {
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

func truncate(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
