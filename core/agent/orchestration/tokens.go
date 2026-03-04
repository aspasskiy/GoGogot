package orchestration

import (
	"gogogot/infra/llm/types"
)

const charsPerToken = 4

// EstimateTokens returns a rough token count for a slice of messages.
// Uses ~4 characters per token heuristic. Sufficient for compaction
// threshold decisions when combined with SafetyMargin.
func EstimateTokens(messages []Message) int {
	var chars int
	for _, m := range messages {
		chars += estimateBlocksChars(m.Content)
	}
	return chars / charsPerToken
}

func estimateBlocksChars(blocks []types.ContentBlock) int {
	var n int
	for _, b := range blocks {
		switch b.Type {
		case "text":
			n += len(b.Text)
		case "tool_use":
			n += len(b.ToolName) + len(b.ToolInput)
		case "tool_result":
			n += len(b.ToolOutput)
		case "image":
			n += 1000 // rough estimate for image tokens
		}
	}
	return n
}
