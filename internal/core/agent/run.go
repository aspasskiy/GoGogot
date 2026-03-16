package agent

import (
	"context"
	"fmt"
	"github.com/aspasskiy/gogogot/internal/core/agent/hook"
	"github.com/aspasskiy/gogogot/internal/core/transport"
	"github.com/aspasskiy/gogogot/internal/llm"
	"github.com/aspasskiy/gogogot/internal/llm/types"
	"github.com/aspasskiy/gogogot/internal/tools/store"
	tooltypes "github.com/aspasskiy/gogogot/internal/tools/types"
	"time"

	"github.com/rs/zerolog/log"
)

// Run executes the agent loop synchronously. Events are sent through the
// provided Bus; the caller is responsible for creating and closing it.
func (a *Agent) Run(ctx context.Context, conv hook.Conversation, userBlocks []types.ContentBlock, bus *transport.Bus) error {
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
			Str("total_cost", fmt.Sprintf("$%.4f", total.Cost)).
			Msg("agent.Run done")
		a.bus.Emit(transport.Done, transport.DoneData{Usage: total})
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
		sys := a.instructions()

		tokensBefore := hook.EstimateTokens(conv.Messages())
		msgCountBefore := len(conv.Messages())

		iterCtx := &hook.IterationContext{
			Iteration:     iteration,
			Model:         a.client.ModelID(),
			System:        sys,
			Messages:      msgs,
			Conversation:  conv,
			ContextWindow: a.client.ContextWindow(),
			LLM:           a.client,
		}
		a.bus.Emit(transport.LLMStart, nil)
		a.runBeforeHooks(ctx, iterCtx)

		if len(conv.Messages()) != msgCountBefore {
			tokensAfter := hook.EstimateTokens(conv.Messages())
			a.bus.Emit(transport.Compaction, transport.CompactionData{
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
			a.bus.Emit(transport.Error, transport.ErrorData{Error: err.Error()})
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
			a.bus.Emit(transport.LLMStream, transport.LLMStreamData{Text: parsed.textContent})
		}

		result := &hook.IterationResult{
			Response:    resp,
			LLMDuration: llmDuration,
		}

		if len(parsed.toolCalls) == 0 {
			a.runAfterHooks(ctx, iterCtx, result)
			if result.Usage != nil {
				a.bus.Emit(transport.LLMResponse, transport.LLMResponseData{Usage: *result.Usage})
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
			a.bus.Emit(transport.LLMResponse, transport.LLMResponseData{Usage: *result.Usage})
		}

		if err := conv.Save(); err != nil {
			log.Error().Err(err).Msg("agent failed to save conversation")
		}
	}

	return nil
}

const maxDetailLen = 60

func (a *Agent) executeToolCallLoop(ctx context.Context, toolCalls []types.ContentBlock, counter *int) ([]types.ContentBlock, []hook.ToolCallSummary) {
	results := make([]types.ContentBlock, 0, len(toolCalls))
	summaries := make([]hook.ToolCallSummary, 0, len(toolCalls))

	toolCtx := transport.WithBus(ctx, a.bus)

	for _, tc := range toolCalls {
		input := unmarshalToolInput(tc.ToolInput)
		tool, _ := a.lookupTool(tc.ToolName)

		label := tool.Label
		if label == "" {
			label = tc.ToolName
		}
		var detail string
		if tool.DetailFunc != nil {
			detail = tool.DetailFunc(input)
			if len(detail) > maxDetailLen {
				detail = detail[:maxDetailLen] + "..."
			}
		}

		a.bus.Emit(transport.ToolStart, transport.ToolStartData{
			Name:   tc.ToolName,
			Label:  label,
			Detail: detail,
			Phase:  tool.Phase,
		})
		*counter++

		if err := a.loopDetector.Check(tc.ToolName, tc.ToolInput); err != nil {
			a.bus.Emit(transport.LoopWarning, transport.LoopWarningData{
				Name: tc.ToolName, Reason: err.Error(),
			})
			results = append(results, types.ToolResultBlock(tc.ToolUseID, err.Error(), true))
			summaries = append(summaries, hook.ToolCallSummary{Name: tc.ToolName, IsErr: true})
			continue
		}

		if tool.Interactive {
			resp, err := a.handleAskUser(ctx, input)
			isErr := err != nil
			output := resp
			if isErr {
				output = err.Error()
			}
			a.bus.Emit(transport.ToolEnd, transport.ToolEndData{Name: tc.ToolName, Result: output})
			results = append(results, types.ToolResultBlock(tc.ToolUseID, output, isErr))
			summaries = append(summaries, hook.ToolCallSummary{Name: tc.ToolName, Duration: 0, IsErr: isErr})
			continue
		}

		start := time.Now()
		toolResult := a.executeTool(toolCtx, tc.ToolName, input)
		elapsed := time.Since(start)

		a.bus.Emit(transport.ToolEnd, transport.ToolEndData{
			Name: tc.ToolName, Result: toolResult.Output, DurationMs: elapsed.Milliseconds(),
		})

		results = append(results, types.ToolResultBlock(tc.ToolUseID, toolResult.Output, toolResult.IsErr))
		summaries = append(summaries, hook.ToolCallSummary{Name: tc.ToolName, Duration: elapsed, IsErr: toolResult.IsErr})
	}

	return results, summaries
}

func (a *Agent) handleAskUser(ctx context.Context, input map[string]any) (string, error) {
	question, _ := input["question"].(string)
	kind := transport.AskKind(tooltypes.GetStringOpt(input, "kind"))
	if kind == "" {
		kind = transport.AskFreeform
	}

	var options []transport.AskOption
	if raw, ok := input["options"].([]any); ok {
		for _, r := range raw {
			if m, ok := r.(map[string]any); ok {
				val, _ := m["value"].(string)
				lbl, _ := m["label"].(string)
				if val != "" && lbl != "" {
					options = append(options, transport.AskOption{Value: val, Label: lbl})
				}
			}
		}
	}

	replyCh := make(chan string, 1)
	if err := a.bus.EmitBlocking(ctx, transport.Ask, transport.AskData{
		Prompt:  question,
		Kind:    kind,
		Options: options,
		ReplyCh: replyCh,
	}); err != nil {
		return "", err
	}

	select {
	case resp := <-replyCh:
		return resp, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}
