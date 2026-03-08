package agent

import (
	"context"
	"time"

	"gogogot/core/agent/event"
	"gogogot/core/agent/hook"
	"gogogot/core/agent/prompt"
	"gogogot/core/agent/session"
	"gogogot/llm"
	"gogogot/llm/types"
	"gogogot/store"

	"github.com/rs/zerolog/log"
)

func (a *Agent) Run(ctx context.Context, userBlocks []types.ContentBlock) error {
	runStart := time.Now()
	log.Info().Str("chat_id", a.Chat.ID).Msg("agent.Run start")
	defer a.logRunDone(runStart)

	a.appendUserMessage(userBlocks)

	var toolCallCounter int

	for iteration := 1; ; iteration++ {
		if err := ctx.Err(); err != nil {
			log.Info().Str("chat_id", a.Chat.ID).Msg("agent.Run cancelled")
			_ = a.Chat.Save()
			return err
		}

		log.Debug().Int("i", iteration).Str("chat_id", a.Chat.ID).Msg("agent loop iteration")

		if err := a.maybeCompact(ctx); err != nil {
			log.Error().Err(err).Msg("compaction failed")
		}

		msgs := a.buildLLMMessages()
		sys := prompt.SystemPrompt(a.config.PromptCtx)

		llmCallCtx := &hook.LLMCallContext{
			Model:    a.client.ModelID(),
			System:   sys,
			Messages: msgs,
		}
		a.emit(event.LLMStart, nil)
		a.runBeforeLLMHooks(ctx, llmCallCtx)

		callStart := time.Now()
		resp, err := a.client.Call(ctx, msgs, llm.CallOptions{
			System:     sys,
			ExtraTools: a.localToolDefs(),
		})
		if err != nil {
			a.emit(event.Error, event.ErrorData{Error: err.Error()})
			return err
		}
		a.runAfterLLMHooks(ctx, llmCallCtx, &hook.LLMCallResult{
			Response: resp,
			Duration: time.Since(callStart),
		})

		usage := a.trackUsage(resp)
		parsed := parseResponseBlocks(resp.Content)
		usage.ToolCalls = len(parsed.toolCalls)

		a.session.Append(session.Message{
			Role:      string(types.RoleAssistant),
			Content:   parsed.assistantBlocks,
			Timestamp: time.Now(),
			Usage:     &usage,
		})

		if parsed.textContent != "" {
			a.Chat.Messages = append(a.Chat.Messages, store.Message{
				Role: string(types.RoleAssistant), Content: parsed.textContent,
			})
			log.Debug().Str("text", parsed.textContent).Msg("agent text response")
			a.emit(event.LLMStream, event.LLMStreamData{Text: parsed.textContent})
		}

		if len(parsed.toolCalls) == 0 {
			log.Debug().Msg("no tool calls, ending agent loop")
			break
		}

		toolResults := a.executeToolCalls(ctx, parsed.toolCalls, &toolCallCounter)
		a.session.Append(session.Message{
			Role:      string(types.RoleUser),
			Content:   toolResults,
			Timestamp: time.Now(),
		})

		if err := a.Chat.Save(); err != nil {
			log.Error().Err(err).Msg("agent failed to save chat")
		}
	}

	return nil
}
