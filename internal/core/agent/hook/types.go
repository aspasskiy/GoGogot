package hook

import (
	"context"
	"time"

	"github.com/aspasskiy/gogogot/internal/domain"
	"github.com/aspasskiy/gogogot/internal/llm"
)

// IterationContext is the per-iteration view passed to hooks. It holds the
// agent's mutable working set of turns — NOT a reference to the chat/store — so
// hooks stay pure: the compaction hook rewrites Messages in place and the agent
// turns that into a persistence event. This is what lets the agent avoid any
// dependency on the store.
type IterationContext struct {
	Iteration     int
	Model         string
	System        string
	Messages      []domain.Turn // mutable; the compaction hook may rewrite this
	ContextWindow int
	LLM           llm.LLM

	// Compacted is set by the compaction hook when it rewrote Messages. The
	// agent reads it to emit a CompactionEvent carrying the new history.
	Compacted bool
}

type IterationResult struct {
	Response    *domain.Response
	LLMDuration time.Duration
	ToolCalls   []ToolCallSummary
	Usage       *domain.Usage
}

type ToolCallSummary struct {
	Name     string
	Duration time.Duration
	IsErr    bool
}

type BeforeIterationFunc func(ctx context.Context, ic *IterationContext)
type AfterIterationFunc func(ctx context.Context, ic *IterationContext, result *IterationResult)
