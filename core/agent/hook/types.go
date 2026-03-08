package hook

import (
	"context"
	"time"

	"gogogot/llm/types"
)

type ToolCallContext struct {
	ToolName  string
	Args      map[string]any
	ArgsRaw   []byte
	CallIndex int
	Timestamp time.Time
}

type ToolCallResult struct {
	Output   string
	IsErr    bool
	Duration time.Duration
}

// BeforeToolCallFunc is called before tool execution.
// Returning a non-nil error blocks execution; the error message
// is sent back to the LLM as the tool result.
type BeforeToolCallFunc func(ctx context.Context, tc *ToolCallContext) error

// AfterToolCallFunc is called after tool execution (observation only).
type AfterToolCallFunc func(ctx context.Context, tc *ToolCallContext, result *ToolCallResult)

type LLMCallContext struct {
	Model    string
	System   string
	Messages []types.Message
}

type LLMCallResult struct {
	Response *types.Response
	Duration time.Duration
}

// BeforeLLMCallFunc is called before an LLM request (observation only).
type BeforeLLMCallFunc func(ctx context.Context, lc *LLMCallContext)

// AfterLLMCallFunc is called after an LLM request completes (observation only).
type AfterLLMCallFunc func(ctx context.Context, lc *LLMCallContext, result *LLMCallResult)
