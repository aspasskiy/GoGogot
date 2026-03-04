package agent

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gogogot/core/agent/orchestration"
	"gogogot/core/store"
	"gogogot/infra/llm"
	"gogogot/infra/llm/types"
	"gogogot/infra/transport"

	"github.com/rs/zerolog/log"
)

func (a *Agent) Run(ctx context.Context, task string, attachments ...transport.Attachment) error {
	runStart := time.Now()
	log.Info().Str("chat_id", a.Chat.ID).Msg("agent.Run start")

	defer func() {
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
		a.emit(orchestration.EventDone, map[string]any{
			"usage": total,
		})
	}()

	var userBlocks []types.ContentBlock
	if len(attachments) > 0 {
		tmpDir := filepath.Join(os.TempDir(), "gogogot-uploads",
			fmt.Sprintf("%s-%d", a.Chat.ID, time.Now().UnixNano()))
		if err := os.MkdirAll(tmpDir, 0755); err != nil {
			log.Error().Err(err).Msg("failed to create upload dir")
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
				log.Error().Err(err).Str("path", fpath).Msg("failed to save attachment")
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
			log.Info().Str("chat_id", a.Chat.ID).Msg("agent.Run cancelled")
			_ = a.Chat.Save()
			return ctx.Err()
		default:
		}

		log.Debug().Int("i", iteration).Str("chat_id", a.Chat.ID).Msg("agent loop iteration")

		if err := a.maybeCompact(ctx); err != nil {
			log.Error().Err(err).Msg("compaction failed")
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
			log.Debug().Int("length", len(textContent)).Msg("agent text response")
			a.emit(orchestration.EventLLMStream, map[string]any{"text": textContent})
		}

		if len(toolCalls) == 0 {
			log.Debug().Msg("no tool calls, ending agent loop")
			break
		}

		log.Debug().Int("count", len(toolCalls)).Msg("agent executing tools")
		var toolResultBlocks []types.ContentBlock
		for _, tc := range toolCalls {
			log.Info().Str("name", tc.ToolName).Int("input_size", len(tc.ToolInput)).Msg("tool call")
			a.emit(orchestration.EventToolStart, map[string]any{"name": tc.ToolName})

			var input map[string]any
			if len(tc.ToolInput) > 0 {
				if err := json.Unmarshal(tc.ToolInput, &input); err != nil {
					log.Error().Err(err).Msg("failed to unmarshal tool input")
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
					log.Warn().Str("tool", tc.ToolName).Str("reason", err.Error()).Msg("before-hook blocked tool call")
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

			log.Info().
				Str("name", tc.ToolName).
				Bool("is_err", result.IsErr).
				Int("output_size", len(result.Output)).
				Dur("duration", elapsed).
				Msg("tool result")
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
			log.Error().Err(err).Msg("agent failed to save chat")
		}
	}

	return nil
}
