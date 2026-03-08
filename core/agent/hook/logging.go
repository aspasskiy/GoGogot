package hook

import (
	"context"

	"github.com/rs/zerolog/log"
)

func LoggingBeforeHook() BeforeToolCallFunc {
	return func(_ context.Context, tc *ToolCallContext) error {
		log.Info().
			Str("name", tc.ToolName).
			Int("input_size", len(tc.ArgsRaw)).
			Msg("tool call")
		return nil
	}
}

func LoggingAfterHook() AfterToolCallFunc {
	return func(_ context.Context, tc *ToolCallContext, result *ToolCallResult) {
		log.Info().
			Str("name", tc.ToolName).
			Bool("is_err", result.IsErr).
			Dur("duration", result.Duration).
			Msg("tool call done")
	}
}
