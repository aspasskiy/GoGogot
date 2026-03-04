package agent

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"gogogot/infra/llm/types"
	"strings"
	"time"

	"gogogot/core/agent/orchestration"
	"gogogot/infra/llm"
	"gogogot/core/store"
	"gogogot/infra/transport"
)

func (a *Agent) Run(ctx context.Context, task string, attachments ...transport.Attachment) error {
	runStart := time.Now()
	slog.Info("agent.Run start", "chat_id", a.Chat.ID)

	defer func() {
		elapsed := time.Since(runStart)
		a.session.TotalUsage.Duration += elapsed
		total := a.session.TotalUsage
		slog.Info("agent.Run done",
			"chat_id", a.Chat.ID,
			"elapsed", elapsed,
			"total_input_tokens", total.InputTokens,
			"total_output_tokens", total.OutputTokens,
			"total_tool_calls", total.ToolCalls,
			"total_cost_usd", total.Cost,
		)
		a.emit(orchestration.EventDone, map[string]any{
			"usage": total,
		})
	}()

	var userBlocks []types.ContentBlock
	if len(attachments) > 0 {
		tmpDir := filepath.Join(os.TempDir(), "gogogot-uploads",
			fmt.Sprintf("%s-%d", a.Chat.ID, time.Now().UnixNano()))
		if err := os.MkdirAll(tmpDir, 0755); err != nil {
			slog.Error("failed to create upload dir", "error", err)
		} else {
			defer os.RemoveAll(tmpDir)
		}

		var imageBlocks []types.ContentBlock
		var paths []string
		nameCount := map[string]int{}

		for _, att := range attachments {
			name := uniqueName(att.Filename, nameCount)
			fpath := filepath.Join(tmpDir, name)
			if err := os.WriteFile(fpath, att.Data, 0644); err != nil {
				slog.Error("failed to save attachment", "path", fpath, "error", err)
				continue
			}
			paths = append(paths, fpath)

			if strings.HasPrefix(att.MimeType, "image/") {
				b64 := base64.StdEncoding.EncodeToString(att.Data)
				imageBlocks = append(imageBlocks, types.ImageBlock(att.MimeType, b64))
			}
		}

		pathList := strings.Join(paths, "\n- ")
		info := fmt.Sprintf("[Attached files saved to disk:\n- %s]", pathList)
		textBlock := task
		if textBlock != "" {
			textBlock += "\n\n" + info
		} else {
			textBlock = info
		}
		userBlocks = append(userBlocks, types.TextBlock(textBlock))
		userBlocks = append(userBlocks, imageBlocks...)
	} else {
		userBlocks = []types.ContentBlock{types.TextBlock(task)}
	}

	a.session.Append(orchestration.Message{
		Role:      "user",
		Content:   userBlocks,
		Timestamp: time.Now(),
	})
	a.Chat.Messages = append(a.Chat.Messages, store.Message{
		Role: "user", Content: task,
	})

	var toolCallCounter int

	for iteration := 1; ; iteration++ {
		select {
		case <-ctx.Done():
			slog.Info("agent.Run cancelled", "chat_id", a.Chat.ID)
			_ = a.Chat.Save()
			return ctx.Err()
		default:
		}

		slog.Debug("agent loop iteration", "i", iteration, "chat_id", a.Chat.ID)

		if err := a.maybeCompact(ctx); err != nil {
			slog.Error("compaction failed", "error", err)
		}

		a.emit(orchestration.EventLLMStart, nil)

		msgs := make([]types.Message, 0, len(a.session.Messages()))
		for _, msg := range a.session.Messages() {
			role := types.RoleUser
			if msg.Role == "assistant" {
				role = types.RoleAssistant
			}
			msgs = append(msgs, types.Message{Role: role, Content: msg.Content})
		}

		resp, err := a.client.Call(ctx, msgs, llm.CallOptions{
			System: a.config.SystemPrompt,
		})
		if err != nil {
			a.emit(orchestration.EventError, map[string]any{"error": err.Error()})
			return err
		}

		usage := orchestration.Usage{
			InputTokens:  resp.InputTokens,
			OutputTokens: resp.OutputTokens,
			LLMCalls:     1,
			Cost:         orchestration.CalcCost(a.client.ModelID(), resp.InputTokens, resp.OutputTokens),
		}

		a.emit(orchestration.EventLLMResponse, map[string]any{"usage": usage})

		var assistantBlocks []types.ContentBlock
		var toolCalls []types.ContentBlock
		var textContent string

		for _, block := range resp.Content {
			switch block.Type {
			case "tool_use":
				toolCalls = append(toolCalls, block)
				assistantBlocks = append(assistantBlocks, block)
			case "text":
				textContent += block.Text
				assistantBlocks = append(assistantBlocks, block)
			}
		}

		usage.ToolCalls = len(toolCalls)

		a.session.Append(orchestration.Message{
			Role:      "assistant",
			Content:   assistantBlocks,
			Timestamp: time.Now(),
			Usage:     &usage,
		})

		if textContent != "" {
			a.Chat.Messages = append(a.Chat.Messages, store.Message{
				Role: "assistant", Content: textContent,
			})
			slog.Debug("agent text response", "length", len(textContent))
			a.emit(orchestration.EventLLMStream, map[string]any{"text": textContent})
		}

		if len(toolCalls) == 0 {
			slog.Debug("no tool calls, ending agent loop")
			break
		}

		slog.Debug("agent executing tools", "count", len(toolCalls))
		var toolResultBlocks []types.ContentBlock
		for _, tc := range toolCalls {
			slog.Info("tool call", "name", tc.ToolName, "input_size", len(tc.ToolInput))
			a.emit(orchestration.EventToolStart, map[string]any{"name": tc.ToolName})

			var input map[string]any
			if len(tc.ToolInput) > 0 {
				if err := json.Unmarshal(tc.ToolInput, &input); err != nil {
					slog.Error("failed to unmarshal tool input", "error", err)
				}
			}

			callCtx := &orchestration.ToolCallContext{
				ToolName:  tc.ToolName,
				Args:      input,
				ArgsRaw:   tc.ToolInput,
				CallIndex: toolCallCounter,
				Timestamp: time.Now(),
			}
			toolCallCounter++

			var blocked bool
			for _, hook := range a.beforeHooks {
				if err := hook(ctx, callCtx); err != nil {
					slog.Warn("before-hook blocked tool call", "tool", tc.ToolName, "reason", err)
					a.emit(orchestration.EventLoopWarning, map[string]any{"name": tc.ToolName, "reason": err.Error()})
					toolResultBlocks = append(toolResultBlocks, types.ToolResultBlock(
						tc.ToolUseID, err.Error(), true,
					))
					blocked = true
					break
				}
			}
			if blocked {
				continue
			}

			start := time.Now()
			result := a.registry.Execute(ctx, tc.ToolName, input)
			elapsed := time.Since(start)

			callResult := &orchestration.ToolCallResult{
				Output:   result.Output,
				IsErr:    result.IsErr,
				Duration: elapsed,
			}
			for _, hook := range a.afterHooks {
				hook(ctx, callCtx, callResult)
			}

			slog.Info("tool result", "name", tc.ToolName, "is_err", result.IsErr, "output_size", len(result.Output), "duration", elapsed)
			a.emit(orchestration.EventToolEnd, map[string]any{"name": tc.ToolName, "result": result.Output, "duration_ms": elapsed.Milliseconds()})

			toolResultBlocks = append(toolResultBlocks, types.ToolResultBlock(
				tc.ToolUseID,
				result.Output,
				result.IsErr,
			))
		}

		a.session.Append(orchestration.Message{
			Role:      "user",
			Content:   toolResultBlocks,
			Timestamp: time.Now(),
		})

		if err := a.Chat.Save(); err != nil {
			slog.Error("agent failed to save chat", "error", err)
		}
	}

	return nil
}
