package hook

import (
	"encoding/json"
	"fmt"
	"github.com/aspasskiy/gogogot/internal/llm/types"
	"strings"
	"time"
)

func CalcCost(inputPerM, outputPerM float64, inputTokens, outputTokens int) float64 {
	return float64(inputTokens)/1_000_000*inputPerM +
		float64(outputTokens)/1_000_000*outputPerM
}

func formatDuration(d time.Duration) string {
	switch {
	case d < time.Second:
		return fmt.Sprintf("%dms", d.Milliseconds())
	case d < time.Minute:
		return fmt.Sprintf("%.1fs", d.Seconds())
	default:
		m := int(d.Minutes())
		s := int(d.Seconds()) % 60
		return fmt.Sprintf("%dm%ds", m, s)
	}
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

const separator = "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

func formatBlocksFull(blocks []types.ContentBlock) string {
	var sb strings.Builder
	for _, b := range blocks {
		switch b.Type {
		case "text":
			sb.WriteString(b.Text)
			sb.WriteByte('\n')
		case "image":
			fmt.Fprintf(&sb, "[image %s, %d bytes]\n", b.MimeType, len(b.ImageData))
		case "tool_use":
			pretty, err := json.MarshalIndent(json.RawMessage(b.ToolInput), "  ", "  ")
			if err != nil {
				pretty = b.ToolInput
			}
			fmt.Fprintf(&sb, "[tool_use: %s]\n  %s\n", b.ToolName, pretty)
		case "tool_result":
			errTag := ""
			if b.ToolIsErr {
				errTag = " ERROR"
			}
			out := b.ToolOutput
			if len(out) > 2000 {
				out = out[:2000] + "… (truncated)"
			}
			fmt.Fprintf(&sb, "[tool_result: %s%s]\n%s\n", b.ToolUseID, errTag, out)
		}
	}
	return sb.String()
}

func formatRequestDump(ic *IterationContext) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "\n%s\n", separator)
	fmt.Fprintf(&sb, "  LLM REQUEST  iter=%d  model=%s\n", ic.Iteration, ic.Model)
	fmt.Fprintf(&sb, "%s\n\n", separator)

	fmt.Fprintf(&sb, "SYSTEM PROMPT:\n%s\n\n", ic.System)

	fmt.Fprintf(&sb, "MESSAGES (%d):\n", len(ic.Messages))
	for i, msg := range ic.Messages {
		fmt.Fprintf(&sb, "── [%d] %s ──\n", i, msg.Role)
		sb.WriteString(formatBlocksFull(msg.Content))
		sb.WriteByte('\n')
	}
	fmt.Fprintf(&sb, "%s\n", separator)
	return sb.String()
}

func formatResponseDump(ic *IterationContext, result *IterationResult) string {
	resp := result.Response
	var sb strings.Builder
	fmt.Fprintf(&sb, "\n%s\n", separator)
	fmt.Fprintf(&sb, "  LLM RESPONSE  iter=%d  stop=%s  %din/%dout  %s\n",
		ic.Iteration, resp.StopReason, resp.InputTokens, resp.OutputTokens, result.LLMDuration)
	fmt.Fprintf(&sb, "%s\n\n", separator)

	sb.WriteString(formatBlocksFull(resp.Content))

	if len(result.ToolCalls) > 0 {
		sb.WriteString("TOOL CALLS:\n")
		for _, tc := range result.ToolCalls {
			errTag := ""
			if tc.IsErr {
				errTag = " ERR"
			}
			fmt.Fprintf(&sb, "  - %s (%s%s)\n", tc.Name, tc.Duration, errTag)
		}
	}

	fmt.Fprintf(&sb, "%s\n", separator)
	return sb.String()
}
