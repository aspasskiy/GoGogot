package agent

import (
	"encoding/json"

	"github.com/aspasskiy/gogogot/internal/domain"

	"github.com/rs/zerolog/log"
)

type parsedResponse struct {
	assistantBlocks []domain.ContentBlock
	toolCalls       []domain.ContentBlock
	textContent     string
}

// buildLLMMessages projects the agent's working set of turns into the message
// form the LLM client expects.
func buildLLMMessages(turns []domain.Turn) []domain.Message {
	msgs := make([]domain.Message, 0, len(turns))
	for _, t := range turns {
		role := domain.RoleUser
		if t.Role == string(domain.RoleAssistant) {
			role = domain.RoleAssistant
		}
		msgs = append(msgs, domain.Message{Role: role, Content: t.Content})
	}
	return msgs
}

func parseResponseBlocks(content []domain.ContentBlock) parsedResponse {
	var p parsedResponse
	for _, block := range content {
		switch block.Type {
		case domain.BlockToolUse:
			p.toolCalls = append(p.toolCalls, block)
			p.assistantBlocks = append(p.assistantBlocks, block)
		case domain.BlockText:
			p.textContent += block.Text
			p.assistantBlocks = append(p.assistantBlocks, block)
		case domain.BlockThinking, domain.BlockRedactedThinking:
			// Kept in order: the adapter replays them on the next iteration.
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
