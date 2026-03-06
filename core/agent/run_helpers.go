package agent

import (
	"context"
	"encoding/json"
	"time"

	"gogogot/core/event"
	"gogogot/core/hooks"
	"gogogot/core/store"
	"gogogot/infra/llm"
	"gogogot/infra/llm/types"

	"github.com/rs/zerolog/log"
)

type parsedResponse struct {
	assistantBlocks []types.ContentBlock
	toolCalls       []types.ContentBlock
	textContent     string
}

func (a *Agent) logRunDone(runStart time.Time) {
	elapsed := time.Since(runStart)
	a.session.TotalUsage.Duration += elapsed
	total := a.session.TotalUsage
	log.Info().
		Str("chat_id", a.Chat.ID).
		Dur("elapsed", elapsed).
		Int("total_input_tokens", total.InputTokens).
		Int("total_output_tokens", total.OutputTokens).
		Int("total_tool_calls", total.ToolCalls).
		Float64("total_cost_usd", total.Cost).
		Msg("agent.Run done")
	a.emit(event.Done, map[string]any{"usage": total})
}

func (a *Agent) appendUserMessage(userBlocks []types.ContentBlock) {
	a.session.Append(Message{
		Role:      string(types.RoleUser),
		Content:   userBlocks,
		Timestamp: time.Now(),
	})

	a.Chat.Messages = append(a.Chat.Messages, store.Message{
		Role: string(types.RoleUser), Content: types.ExtractText(userBlocks),
	})
}

func (a *Agent) buildLLMMessages() []types.Message {
	sessionMsgs := a.session.Messages()
	msgs := make([]types.Message, 0, len(sessionMsgs))
	for _, msg := range sessionMsgs {
		role := types.RoleUser
		if msg.Role == string(types.RoleAssistant) {
			role = types.RoleAssistant
		}
		msgs = append(msgs, types.Message{Role: role, Content: msg.Content})
	}
	return msgs
}

func (a *Agent) trackUsage(resp *llm.Response) Usage {
	usage := Usage{
		InputTokens:  resp.InputTokens,
		OutputTokens: resp.OutputTokens,
		LLMCalls:     1,
		Cost:         llm.CalcCost(a.client.ModelID(), resp.InputTokens, resp.OutputTokens),
	}
	a.emit(event.LLMResponse, map[string]any{"usage": usage})
	return usage
}

func parseResponseBlocks(content []types.ContentBlock) parsedResponse {
	var p parsedResponse
	for _, block := range content {
		switch block.Type {
		case "tool_use":
			p.toolCalls = append(p.toolCalls, block)
			p.assistantBlocks = append(p.assistantBlocks, block)
		case "text":
			p.textContent += block.Text
			p.assistantBlocks = append(p.assistantBlocks, block)
		}
	}
	return p
}

func (a *Agent) executeToolCalls(ctx context.Context, toolCalls []types.ContentBlock, counter *int) []types.ContentBlock {
	results := make([]types.ContentBlock, 0, len(toolCalls))
	for _, tc := range toolCalls {
		result := a.executeSingleTool(ctx, tc, counter)
		results = append(results, result)
	}
	return results
}

func (a *Agent) executeSingleTool(ctx context.Context, tc types.ContentBlock, counter *int) types.ContentBlock {
	a.emit(event.ToolStart, map[string]any{"name": tc.ToolName})

	var input map[string]any
	if len(tc.ToolInput) > 0 {
		if err := json.Unmarshal(tc.ToolInput, &input); err != nil {
			log.Error().Err(err).Msg("failed to unmarshal tool input")
		}
	}

	callCtx := &hooks.ToolCallContext{
		ToolName:  tc.ToolName,
		Args:      input,
		ArgsRaw:   tc.ToolInput,
		CallIndex: *counter,
		Timestamp: time.Now(),
	}
	*counter++

	if err := a.runBeforeHooks(ctx, callCtx); err != nil {
		a.emit(event.LoopWarning, map[string]any{"name": tc.ToolName, "reason": err.Error()})
		return types.ToolResultBlock(tc.ToolUseID, err.Error(), true)
	}

	start := time.Now()
	result, handled := a.executeLocal(ctx, tc.ToolName, input)
	if !handled {
		result = a.registry.Execute(ctx, tc.ToolName, input)
	}
	elapsed := time.Since(start)

	a.runAfterHooks(ctx, callCtx, &hooks.ToolCallResult{
		Output:   result.Output,
		IsErr:    result.IsErr,
		Duration: elapsed,
	})

	a.emit(event.ToolEnd, map[string]any{
		"name": tc.ToolName, "result": result.Output, "duration_ms": elapsed.Milliseconds(),
	})

	return types.ToolResultBlock(tc.ToolUseID, result.Output, result.IsErr)
}

func (a *Agent) runBeforeHooks(ctx context.Context, callCtx *hooks.ToolCallContext) error {
	for _, hook := range a.beforeHooks {
		if err := hook(ctx, callCtx); err != nil {
			return err
		}
	}
	return nil
}

func (a *Agent) runAfterHooks(ctx context.Context, callCtx *hooks.ToolCallContext, result *hooks.ToolCallResult) {
	for _, hook := range a.afterHooks {
		hook(ctx, callCtx, result)
	}
}
