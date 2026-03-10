package agent

import (
	"context"
	event2 "gogogot/internal/core/agent/event"
	"gogogot/internal/core/agent/hook"
	"gogogot/internal/core/prompt"
	"gogogot/internal/llm"
	"gogogot/internal/llm/types"
	"gogogot/internal/tools/store"
	"time"

	"github.com/rs/zerolog/log"
)

// Run executes the agent loop synchronously. Events are sent through the
// provided Bus; the caller is responsible for creating and closing it.
func (a *Agent) Run(ctx context.Context, conv hook.Conversation, userBlocks []types.ContentBlock, bus *event2.Bus) error {
	a.bus = bus

	runStart := time.Now()
	log.Info().Str("conversation", conv.String()).Msg("agent.Run start")
	defer func() {
		elapsed := time.Since(runStart)
		conv.TotalUsage().Duration += elapsed
		total := *conv.TotalUsage()
		log.Info().
			Str("conversation", conv.String()).
			Dur("elapsed", elapsed).
			Int("total_input_tokens", total.InputTokens).
			Int("total_output_tokens", total.OutputTokens).
			Int("total_tool_calls", total.ToolCalls).
			Float64("total_cost_usd", total.Cost).
			Msg("agent.Run done")
		a.bus.Emit(event2.Done, event2.DoneData{Usage: total})
	}()

	appendUserMessage(conv, userBlocks)
	conv.Save()

	var toolCallCounter int

	for iteration := 1; ; iteration++ {
		if err := ctx.Err(); err != nil {
			log.Info().Str("conversation", conv.String()).Msg("agent.Run cancelled")
			_ = conv.Save()
			return err
		}

		msgs := buildLLMMessages(conv)
		sys := prompt.SystemPrompt(a.config.PromptLoader())

		tokensBefore := store.EstimateTokens(conv.Messages())
		msgCountBefore := len(conv.Messages())

		iterCtx := &hook.IterationContext{
			Iteration:     iteration,
			Model:         a.client.ModelID(),
			System:        sys,
			Messages:      msgs,
			Conversation:  conv,
			ContextWindow: a.client.ContextWindow(),
		}
		a.bus.Emit(event2.LLMStart, nil)
		a.runBeforeHooks(ctx, iterCtx)

		if len(conv.Messages()) != msgCountBefore {
			tokensAfter := store.EstimateTokens(conv.Messages())
			a.bus.Emit(event2.Compaction, event2.CompactionData{
				BeforeTokens: tokensBefore,
				AfterTokens:  tokensAfter,
			})
		}

		msgs = buildLLMMessages(conv)
		iterCtx.Messages = msgs

		callStart := time.Now()
		resp, err := a.client.Call(ctx, msgs, llm.CallOptions{
			System:     sys,
			ExtraTools: a.localToolDefs(),
		})
		if err != nil {
			a.bus.Emit(event2.Error, event2.ErrorData{Error: err.Error()})
			return err
		}
		llmDuration := time.Since(callStart)

		parsed := parseResponseBlocks(resp.Content)

		conv.AppendMessage(store.Turn{
			Role:      string(types.RoleAssistant),
			Content:   parsed.assistantBlocks,
			Timestamp: time.Now(),
		})

		if parsed.textContent != "" {
			log.Debug().Str("text", parsed.textContent).Msg("agent text response")
			a.bus.Emit(event2.LLMStream, event2.LLMStreamData{Text: parsed.textContent})
		}

		result := &hook.IterationResult{
			Response:    resp,
			LLMDuration: llmDuration,
		}

		if len(parsed.toolCalls) == 0 {
			log.Debug().Msg("no tool calls, ending agent loop")
			a.runAfterHooks(ctx, iterCtx, result)
			if result.Usage != nil {
				a.bus.Emit(event2.LLMResponse, event2.LLMResponseData{Usage: *result.Usage})
			}
			break
		}

		// --- tool-call loop (single emission point for tool events) ---
		toolResults, summaries := a.executeToolCallLoop(ctx, parsed.toolCalls, &toolCallCounter)
		result.ToolCalls = summaries

		conv.AppendMessage(store.Turn{
			Role:      string(types.RoleUser),
			Content:   toolResults,
			Timestamp: time.Now(),
		})

		a.runAfterHooks(ctx, iterCtx, result)
		if result.Usage != nil {
			a.bus.Emit(event2.LLMResponse, event2.LLMResponseData{Usage: *result.Usage})
		}

		if err := conv.Save(); err != nil {
			log.Error().Err(err).Msg("agent failed to save conversation")
		}
	}

	return nil
}

func (a *Agent) executeToolCallLoop(ctx context.Context, toolCalls []types.ContentBlock, counter *int) ([]types.ContentBlock, []hook.ToolCallSummary) {
	results := make([]types.ContentBlock, 0, len(toolCalls))
	summaries := make([]hook.ToolCallSummary, 0, len(toolCalls))

	for _, tc := range toolCalls {
		input := unmarshalToolInput(tc.ToolInput)

		a.bus.Emit(event2.ToolStart, event2.ToolStartData{
			Name:   tc.ToolName,
			Detail: extractToolDetail(tc.ToolName, input),
		})
		*counter++

		if err := a.loopDetector.Check(tc.ToolName, tc.ToolInput); err != nil {
			a.bus.Emit(event2.LoopWarning, event2.LoopWarningData{
				Name: tc.ToolName, Reason: err.Error(),
			})
			results = append(results, types.ToolResultBlock(tc.ToolUseID, err.Error(), true))
			summaries = append(summaries, hook.ToolCallSummary{Name: tc.ToolName, IsErr: true})
			continue
		}

		start := time.Now()
		toolResult := a.executeTool(ctx, tc.ToolName, input)
		elapsed := time.Since(start)

		log.Info().
			Str("name", tc.ToolName).
			Bool("is_err", toolResult.IsErr).
			Dur("duration", elapsed).
			Msg("tool call done")

		a.bus.Emit(event2.ToolEnd, event2.ToolEndData{
			Name: tc.ToolName, Result: toolResult.Output, DurationMs: elapsed.Milliseconds(),
		})

		results = append(results, types.ToolResultBlock(tc.ToolUseID, toolResult.Output, toolResult.IsErr))
		summaries = append(summaries, hook.ToolCallSummary{Name: tc.ToolName, Duration: elapsed, IsErr: toolResult.IsErr})
	}

	return results, summaries
}

