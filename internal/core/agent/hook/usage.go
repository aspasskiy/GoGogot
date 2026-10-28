package hook

import (
	"context"

	"github.com/aspasskiy/gogogot/internal/domain"
)

// UsageAfterIteration computes this iteration's token/cost usage and stores it
// on the result. The agent accumulates result.Usage across iterations and emits
// the total in DoneEvent; the orchestrator folds it into the chat. The hook
// itself stays pure — it no longer mutates any conversation state.
func UsageAfterIteration(pricing domain.Pricing) AfterIterationFunc {
	return func(_ context.Context, _ *IterationContext, result *IterationResult) {
		resp := result.Response
		usage := domain.Usage{
			InputTokens:      resp.InputTokens,
			OutputTokens:     resp.OutputTokens,
			CacheReadTokens:  resp.CacheReadTokens,
			CacheWriteTokens: resp.CacheWriteTokens,
			LLMCalls:         1,
			ToolCalls:        len(result.ToolCalls),
			Cost:             pricing.Cost(resp),
		}
		result.Usage = &usage
	}
}
