package llm

import "strings"

type modelPricing struct {
	InputPerM  float64
	OutputPerM float64
}

var knownPricing = map[string]modelPricing{
	"claude-sonnet-4-6":    {InputPerM: 3.0, OutputPerM: 15.0},
	"minimax/minimax-m2.5": {InputPerM: 0.5, OutputPerM: 1.5},
}

// CalcCost estimates USD cost for a single LLM call.
// Falls back to zero if model is unknown.
func CalcCost(model string, inputTokens, outputTokens int) float64 {
	p, ok := knownPricing[model]
	if !ok {
		for k, v := range knownPricing {
			if strings.Contains(model, k) || strings.Contains(k, model) {
				p = v
				ok = true
				break
			}
		}
	}
	if !ok {
		return 0
	}
	return float64(inputTokens)/1_000_000*p.InputPerM +
		float64(outputTokens)/1_000_000*p.OutputPerM
}
