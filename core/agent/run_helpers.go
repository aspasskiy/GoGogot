package agent

import (
	"context"
	"encoding/json"
	"net/url"
	"path/filepath"
	"time"

	"gogogot/store"
	"gogogot/llm"
	"gogogot/llm/types"

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
	a.emit(EventDone, DoneData{Usage: total})
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
	a.emit(EventLLMResponse, LLMResponseData{Usage: usage})
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
	var input map[string]any
	if len(tc.ToolInput) > 0 {
		if err := json.Unmarshal(tc.ToolInput, &input); err != nil {
			log.Error().Err(err).Msg("failed to unmarshal tool input")
		}
	}

	a.emit(EventToolStart, ToolStartData{
		Name:   tc.ToolName,
		Detail: extractToolDetail(tc.ToolName, input),
	})

	callCtx := &ToolCallContext{
		ToolName:  tc.ToolName,
		Args:      input,
		ArgsRaw:   tc.ToolInput,
		CallIndex: *counter,
		Timestamp: time.Now(),
	}
	*counter++

	if err := a.runBeforeHooks(ctx, callCtx); err != nil {
		a.emit(EventLoopWarning, LoopWarningData{Name: tc.ToolName, Reason: err.Error()})
		return types.ToolResultBlock(tc.ToolUseID, err.Error(), true)
	}

	start := time.Now()
	result, handled := a.executeLocal(ctx, tc.ToolName, input)
	if !handled {
		result = a.registry.Execute(ctx, tc.ToolName, input)
	}
	elapsed := time.Since(start)

	a.runAfterHooks(ctx, callCtx, &ToolCallResult{
		Output:   result.Output,
		IsErr:    result.IsErr,
		Duration: elapsed,
	})

	a.emit(EventToolEnd, ToolEndData{
		Name: tc.ToolName, Result: result.Output, DurationMs: elapsed.Milliseconds(),
	})

	return types.ToolResultBlock(tc.ToolUseID, result.Output, result.IsErr)
}

func (a *Agent) runBeforeHooks(ctx context.Context, callCtx *ToolCallContext) error {
	for _, hook := range a.beforeHooks {
		if err := hook(ctx, callCtx); err != nil {
			return err
		}
	}
	return nil
}

func (a *Agent) runAfterHooks(ctx context.Context, callCtx *ToolCallContext, result *ToolCallResult) {
	for _, hook := range a.afterHooks {
		hook(ctx, callCtx, result)
	}
}

const maxDetailLen = 60

func extractToolDetail(name string, input map[string]any) string {
	var detail string
	switch name {
	case "bash":
		detail, _ = input["command"].(string)
	case "edit_file", "read_file", "write_file":
		if p, ok := input["path"].(string); ok {
			detail = filepath.Base(p)
		}
	case "web_search":
		detail, _ = input["query"].(string)
	case "web_fetch":
		if raw, ok := input["url"].(string); ok {
			if u, err := url.Parse(raw); err == nil {
				detail = u.Host
			}
		}
	}
	if len(detail) > maxDetailLen {
		detail = detail[:maxDetailLen] + "..."
	}
	return detail
}
