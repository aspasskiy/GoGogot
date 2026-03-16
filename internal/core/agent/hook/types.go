package hook

import (
	"context"
	"fmt"
	"github.com/aspasskiy/gogogot/internal/llm"
	"github.com/aspasskiy/gogogot/internal/llm/types"
	"github.com/aspasskiy/gogogot/internal/tools/store"
	"time"
)

// Conversation is the agent's view of a conversation store.
// Decouples the agent loop from the concrete store.Episode implementation.
type Conversation interface {
	fmt.Stringer
	Messages() []store.Turn
	AppendMessage(store.Turn)
	ReplaceMessages([]store.Turn) error
	TotalUsage() *store.Usage
	Save() error
}

type IterationContext struct {
	Iteration     int
	Model         string
	System        string
	Messages      []types.Message
	Conversation  Conversation
	ContextWindow int
	LLM           llm.LLM
}

type IterationResult struct {
	Response    *types.Response
	LLMDuration time.Duration
	ToolCalls   []ToolCallSummary
	Usage       *store.Usage
}

type ToolCallSummary struct {
	Name     string
	Duration time.Duration
	IsErr    bool
}

type BeforeIterationFunc func(ctx context.Context, ic *IterationContext)
type AfterIterationFunc func(ctx context.Context, ic *IterationContext, result *IterationResult)
