package hook

import (
	"context"
	"fmt"
	"strings"

	"github.com/aspasskiy/gogogot/internal/domain"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func LoggingBeforeIteration() BeforeIterationFunc {
	return func(_ context.Context, ic *IterationContext) {
		log.Info().
			Int("iteration", ic.Iteration).
			Str("model", ic.Model).
			Int("messages", len(ic.Messages)).
			Msg("iteration start")

		if log.Logger.GetLevel() <= zerolog.TraceLevel {
			log.Trace().Msg(formatRequestDump(ic))
		}
	}
}

func LoggingAfterIteration(pricing domain.Pricing) AfterIterationFunc {
	return func(_ context.Context, ic *IterationContext, result *IterationResult) {
		resp := result.Response
		cost := pricing.Cost(resp)
		evt := log.Info().
			Int("iteration", ic.Iteration).
			Str("elapsed", formatDuration(result.LLMDuration)).
			Int("in_tokens", resp.InputTokens).
			Int("cache_read", resp.CacheReadTokens).
			Int("cache_write", resp.CacheWriteTokens).
			Int("out_tokens", resp.OutputTokens).
			Str("cost", fmt.Sprintf("$%.4f", cost)).
			Str("stop", resp.StopReason)

		if len(result.ToolCalls) > 0 {
			names := make([]string, 0, len(result.ToolCalls))
			for _, tc := range result.ToolCalls {
				names = append(names, tc.Name)
			}
			evt.Str("tools", strings.Join(names, ", "))
		}

		evt.Msg("iteration done")

		if log.Logger.GetLevel() <= zerolog.TraceLevel {
			log.Trace().Msg(formatResponseDump(ic, result))
		}
	}
}
