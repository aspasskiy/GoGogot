package episode

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/aspasskiy/gogogot/internal/llm"
	"github.com/aspasskiy/gogogot/internal/llm/types"
	"github.com/aspasskiy/gogogot/internal/tools/store"
)

type decision string

const (
	decisionSame decision = "same"
	decisionNew  decision = "new"
)

const classifySystem = `You decide whether a new user message belongs to the current conversation or starts a brand-new topic.
Reply with ONLY valid JSON: {"decision":"same"} or {"decision":"new"}.
No explanation, no extra text.`

const (
	classifyTimeGap    = 2 * time.Hour
	classifyMinMsgSkip = 2
	classifyMaxTokens  = 30
)

func (m *Manager) classify(ctx context.Context, ep *store.Episode, newMessage string) (decision, error) {
	if time.Since(ep.UpdatedAt) > classifyTimeGap {
		return decisionNew, nil
	}

	if ep.UserMsgCount <= classifyMinMsgSkip {
		return decisionSame, nil
	}

	var contextBlock string
	if ep.RunSummary != "" {
		contextBlock = "Conversation summary so far:\n" + ep.RunSummary
	} else {
		messages, err := ep.TextMessages()
		if err != nil || len(messages) == 0 {
			return decisionNew, nil
		}
		const recentMessagesCap = 6
		if len(messages) > recentMessagesCap {
			messages = messages[len(messages)-recentMessagesCap:]
		}
		var transcript strings.Builder
		for _, msg := range messages {
			fmt.Fprintf(&transcript, "[%s]: %s\n", msg.Role, msg.Content)
		}
		contextBlock = "Current conversation (last messages):\n" + transcript.String()
	}

	prompt := fmt.Sprintf("%s\n---\nNew user message:\n%s", contextBlock, newMessage)

	resp, err := m.llm.Call(ctx, []types.Message{
		types.NewUserMessage(types.TextBlock(prompt)),
	}, llm.CallOptions{
		System:    classifySystem,
		NoTools:   true,
		MaxTokens: classifyMaxTokens,
	})
	if err != nil {
		return "", fmt.Errorf("classify LLM call: %w", err)
	}

	return parseClassifyResponse(types.ExtractText(resp.Content))
}

func parseClassifyResponse(text string) (decision, error) {
	text = strings.TrimSpace(text)

	var result struct {
		Decision string `json:"decision"`
	}
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		start := strings.Index(text, "{")
		end := strings.LastIndex(text, "}")
		if start >= 0 && end > start {
			_ = json.Unmarshal([]byte(text[start:end+1]), &result)
		}
	}

	switch decision(strings.ToLower(result.Decision)) {
	case decisionSame:
		return decisionSame, nil
	case decisionNew:
		return decisionNew, nil
	default:
		return decisionSame, fmt.Errorf("unexpected classification %q, defaulting to same", result.Decision)
	}
}
