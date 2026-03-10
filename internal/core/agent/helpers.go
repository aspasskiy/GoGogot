package agent

import (
	"encoding/json"
	"gogogot/internal/core/agent/hook"
	"gogogot/internal/llm/types"
	"gogogot/internal/tools/store"
	"net/url"
	"path/filepath"
	"time"

	"github.com/rs/zerolog/log"
)

type parsedResponse struct {
	assistantBlocks []types.ContentBlock
	toolCalls       []types.ContentBlock
	textContent     string
}

func appendUserMessage(conv hook.Conversation, userBlocks []types.ContentBlock) {
	conv.AppendMessage(store.Turn{
		Role:      string(types.RoleUser),
		Content:   userBlocks,
		Timestamp: time.Now(),
	})
}

func buildLLMMessages(conv hook.Conversation) []types.Message {
	turns := conv.Messages()
	msgs := make([]types.Message, 0, len(turns))
	for _, t := range turns {
		role := types.RoleUser
		if t.Role == string(types.RoleAssistant) {
			role = types.RoleAssistant
		}
		msgs = append(msgs, types.Message{Role: role, Content: t.Content})
	}
	return msgs
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

func unmarshalToolInput(raw json.RawMessage) map[string]any {
	var input map[string]any
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &input); err != nil {
			log.Error().Err(err).Msg("failed to unmarshal tool input")
		}
	}
	return input
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
