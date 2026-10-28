package agent

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aspasskiy/gogogot/internal/core/agent/hook"
	"github.com/aspasskiy/gogogot/internal/domain"
	"github.com/aspasskiy/gogogot/internal/llm"
	tooltypes "github.com/aspasskiy/gogogot/internal/tools/types"
	"github.com/aspasskiy/gogogot/internal/transport"

	"github.com/rs/zerolog/log"
)

// RunInput is everything the agent needs for one run. Messages is the prior
// history (treated as immutable); UserBlocks is the new user message. The agent
// emits every turn it produces — starting with the user turn — as a
// MessageEvent, leaving persistence entirely to the sink.
type RunInput struct {
	Messages   []domain.Turn
	UserBlocks []domain.ContentBlock
}

// Run executes the agent loop synchronously, emitting typed events to the sink.
// It never touches the store: persistence is the sink's responsibility.
//
// Exactly one terminal event is emitted: DoneEvent on success or ErrorEvent on
// a hard failure. On cancellation it emits neither and returns ctx.Err() — the
// orchestrator closing the event channel is the UI's cleanup signal.
func (a *Agent) Run(ctx context.Context, input RunInput, sink transport.Sink) error {
	return a.newRun(sink).execute(ctx, input)
}

func (r *run) execute(ctx context.Context, input RunInput) error {
	runStart := time.Now()
	log.Info().Int("history", len(input.Messages)).Msg("agent.Run start")

	// The new user message is persisted like any other turn — the orchestrator
	// writes it when it sees this event.
	userTurn := domain.Turn{
		Role:      string(domain.RoleUser),
		Content:   input.UserBlocks,
		Timestamp: time.Now(),
	}
	r.emit(transport.MessageEvent{Turn: userTurn})

	msgs := make([]domain.Turn, 0, len(input.Messages)+1)
	msgs = append(msgs, input.Messages...)
	msgs = append(msgs, userTurn)

	var total domain.Usage
	var finalText string // last assistant text produced; reported in DoneEvent
	var toolCallCounter int

	// Build the system prompt once per run: an unchanged prefix across
	// iterations is what lets the provider serve it from the prompt cache.
	// Changes made mid-run (new skills, identity edits) apply from the next run.
	sys := r.instructions()

	for iteration := 1; ; iteration++ {
		if err := ctx.Err(); err != nil {
			log.Info().Msg("agent.Run cancelled")
			return err
		}

		tokensBefore := hook.EstimateTokens(msgs)

		iterCtx := &hook.IterationContext{
			Iteration:     iteration,
			Model:         r.client.ModelID(),
			System:        sys,
			Messages:      msgs,
			ContextWindow: r.client.ContextWindow(),
			LLM:           r.client,
		}
		r.emit(transport.LLMStartEvent{})
		r.runBeforeHooks(ctx, iterCtx)

		// The compaction hook may have rewritten the working set in place.
		if iterCtx.Compacted {
			msgs = iterCtx.Messages
			r.emit(transport.CompactionEvent{
				Messages:     append([]domain.Turn(nil), msgs...), // copy: the sink takes ownership
				BeforeTokens: tokensBefore,
				AfterTokens:  hook.EstimateTokens(msgs),
			})
		}

		callStart := time.Now()
		resp, err := r.client.Call(ctx, buildLLMMessages(msgs), llm.CallOptions{
			System:     sys,
			ExtraTools: r.localToolDefs,
		})
		if err != nil {
			// Clean cancellation (user /stop or shutdown): exit quietly, no event.
			if errors.Is(err, context.Canceled) {
				log.Info().Msg("agent.Run cancelled during LLM call")
				return err
			}
			msg := err.Error()
			if errors.Is(err, context.DeadlineExceeded) {
				msg = "LLM provider did not respond in time. The model may be overloaded — try again later."
			}
			r.emit(transport.ErrorEvent{Error: msg})
			return err
		}
		llmDuration := time.Since(callStart)

		parsed := parseResponseBlocks(resp.Content)

		assistantTurn := domain.Turn{
			Role:      string(domain.RoleAssistant),
			Content:   parsed.assistantBlocks,
			Timestamp: time.Now(),
		}
		r.emit(transport.MessageEvent{Turn: assistantTurn})
		msgs = append(msgs, assistantTurn)

		if parsed.textContent != "" {
			finalText = parsed.textContent
			r.emit(transport.LLMStreamEvent{Text: parsed.textContent})
		}

		result := &hook.IterationResult{
			Response:    resp,
			LLMDuration: llmDuration,
		}

		if len(parsed.toolCalls) == 0 {
			r.runAfterHooks(ctx, iterCtx, result)
			if result.Usage != nil {
				total.Add(*result.Usage)
			}
			break
		}

		// --- tool-call loop (single emission point for tool events) ---
		toolResults, summaries := r.executeToolCallLoop(ctx, parsed.toolCalls, &toolCallCounter)
		result.ToolCalls = summaries

		toolTurn := domain.Turn{
			Role:      string(domain.RoleUser),
			Content:   toolResults,
			Timestamp: time.Now(),
		}
		r.emit(transport.MessageEvent{Turn: toolTurn})
		msgs = append(msgs, toolTurn)

		r.runAfterHooks(ctx, iterCtx, result)
		if result.Usage != nil {
			total.Add(*result.Usage)
		}
	}

	total.Duration = time.Since(runStart)
	log.Info().
		Dur("elapsed", total.Duration).
		Int("total_input_tokens", total.InputTokens).
		Int("total_output_tokens", total.OutputTokens).
		Int("total_tool_calls", total.ToolCalls).
		Str("total_cost", fmt.Sprintf("$%.4f", total.Cost)).
		Msg("agent.Run done")
	r.emit(transport.DoneEvent{Text: finalText, Usage: total})
	return nil
}

const maxDetailLen = 60

func (r *run) executeToolCallLoop(ctx context.Context, toolCalls []domain.ContentBlock, counter *int) ([]domain.ContentBlock, []hook.ToolCallSummary) {
	results := make([]domain.ContentBlock, 0, len(toolCalls))
	summaries := make([]hook.ToolCallSummary, 0, len(toolCalls))

	toolCtx := transport.WithSink(ctx, r.sink)

	for _, tc := range toolCalls {
		input := unmarshalToolInput(tc.ToolInput)
		tool, _ := r.lookupTool(tc.ToolName)

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

		r.emit(transport.ToolStartEvent{
			Name:   tc.ToolName,
			Label:  label,
			Detail: detail,
			Phase:  tool.Phase,
		})
		*counter++

		if err := r.loopDetector.Check(tc.ToolName, tc.ToolInput); err != nil {
			r.emit(transport.LoopWarningEvent{
				Name: tc.ToolName, Reason: err.Error(),
			})
			results = append(results, domain.ToolResultBlock(tc.ToolUseID, err.Error(), true))
			summaries = append(summaries, hook.ToolCallSummary{Name: tc.ToolName, IsErr: true})
			continue
		}

		if tool.Interactive {
			resp, err := r.handleAskUser(ctx, input)
			isErr := err != nil
			output := resp
			if isErr {
				output = err.Error()
			}
			r.emit(transport.ToolEndEvent{Name: tc.ToolName, Result: output})
			results = append(results, domain.ToolResultBlock(tc.ToolUseID, output, isErr))
			summaries = append(summaries, hook.ToolCallSummary{Name: tc.ToolName, Duration: 0, IsErr: isErr})
			continue
		}

		start := time.Now()
		toolResult := r.executeTool(toolCtx, tc.ToolName, input)
		elapsed := time.Since(start)

		r.emit(transport.ToolEndEvent{
			Name: tc.ToolName, Result: toolResult.Output, DurationMs: elapsed.Milliseconds(),
		})

		results = append(results, domain.ToolResultBlock(tc.ToolUseID, toolResult.Output, toolResult.IsErr))
		summaries = append(summaries, hook.ToolCallSummary{Name: tc.ToolName, Duration: elapsed, IsErr: toolResult.IsErr})
	}

	return results, summaries
}

func (r *run) handleAskUser(ctx context.Context, input map[string]any) (string, error) {
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

	// The sink delivers AskEvent with a guarantee (it must not be dropped, or
	// the agent would block here forever). If the run is cancelled the event is
	// abandoned and the ctx.Done branch below unblocks us.
	replyCh := make(chan string, 1)
	r.emit(transport.AskEvent{
		Prompt:  question,
		Kind:    kind,
		Options: options,
		ReplyCh: replyCh,
	})

	select {
	case resp := <-replyCh:
		return resp, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}
