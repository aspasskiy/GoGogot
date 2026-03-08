package session

import (
	"time"

	"gogogot/llm/types"
)

type Usage struct {
	InputTokens      int
	OutputTokens     int
	CacheReadTokens  int
	CacheWriteTokens int
	TotalTokens      int
	LLMCalls         int
	ToolCalls        int
	Cost             float64 // estimated USD
	Duration         time.Duration
}

func (u *Usage) Add(other Usage) {
	u.InputTokens += other.InputTokens
	u.OutputTokens += other.OutputTokens
	u.CacheReadTokens += other.CacheReadTokens
	u.CacheWriteTokens += other.CacheWriteTokens
	u.TotalTokens += other.TotalTokens
	u.LLMCalls += other.LLMCalls
	u.ToolCalls += other.ToolCalls
	u.Cost += other.Cost
	u.Duration += other.Duration
}

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
			n += 1000
		}
	}
	return n
}
