package agent

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"
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
