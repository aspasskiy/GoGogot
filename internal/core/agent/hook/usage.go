package hook

import (
	"context"
	"github.com/aspasskiy/gogogot/internal/tools/store"
)

func UsageAfterIteration(inputPricePerM, outputPricePerM float64) AfterIterationFunc {
	return func(_ context.Context, ic *IterationContext, result *IterationResult) {
		resp := result.Response
		usage := store.Usage{
			InputTokens:  resp.InputTokens,
			OutputTokens: resp.OutputTokens,
			LLMCalls:     1,
			ToolCalls:    len(result.ToolCalls),
			Cost:         CalcCost(inputPricePerM, outputPricePerM, resp.InputTokens, resp.OutputTokens),
		}
		result.Usage = &usage
		ic.Conversation.TotalUsage().Add(usage)
	}
}
