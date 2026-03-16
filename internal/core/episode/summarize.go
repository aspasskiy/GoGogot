package episode

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/aspasskiy/gogogot/internal/infra/utils"
	"github.com/aspasskiy/gogogot/internal/llm"
	"github.com/aspasskiy/gogogot/internal/llm/types"
	"github.com/aspasskiy/gogogot/internal/tools/store"

	"github.com/rs/zerolog/log"
)

const (
	summaryInterval       = 5
	runSummaryMaxTokens   = 150
	closeSummaryMaxTokens = 300
)

type summaryResult struct {
	Title   string   `json:"title"`
	Summary string   `json:"summary"`
	Tags    []string `json:"tags"`
}

func TruncTitle(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	return utils.Truncate(s, 60, "...")
}

// Close summarizes the episode via the LLM, then marks it closed and saves.
func (m *Manager) Close(ctx context.Context, ep *store.Episode) error {
	messages, err := ep.TextMessages()
	if err != nil || len(messages) == 0 {
		ep.Close()
		return ep.Save()
	}

	var contextPart string
	if ep.RunSummary != "" {
		const tailCap = 4
		tail := messages
		if len(tail) > tailCap {
			tail = tail[len(tail)-tailCap:]
		}
		var recent strings.Builder
		for _, msg := range tail {
			fmt.Fprintf(&recent, "[%s]: %s\n", msg.Role, msg.Content)
		}
		contextPart = "Running summary:\n" + ep.RunSummary +
			"\n\nRecent messages:\n" + recent.String()
	} else {
		var transcript strings.Builder
		for _, msg := range messages {
			fmt.Fprintf(&transcript, "[%s]: %s\n", msg.Role, msg.Content)
		}
		contextPart = transcript.String()
	}

	prompt := "Summarize this conversation episode. Return ONLY valid JSON:\n" +
		`{"title": "short title", "summary": "2-3 sentence summary", "tags": ["tag1", "tag2"]}` +
		"\n\nPreserve: key decisions, outcomes, important facts, action items.\n\n---\n\n" +
		contextPart

	resp, err := m.llm.Call(ctx, []types.Message{
		types.NewUserMessage(types.TextBlock(prompt)),
	}, llm.CallOptions{
		System:    "You summarize conversations into structured JSON. Be concise and accurate.",
		NoTools:   true,
		MaxTokens: closeSummaryMaxTokens,
	})

	if err != nil {
		log.Error().Err(err).Str("episode", ep.ID).Msg("episode: summarization failed")
		if len(messages) > 0 {
			ep.Title = TruncTitle(messages[0].Content)
		}
		ep.Summary = "(summarization failed)"
	} else {
		text := types.ExtractText(resp.Content)
		result := parseSummaryJSON(text)
		if result.Title != "" {
			ep.Title = result.Title
		} else if ep.Title == "" && len(messages) > 0 {
			ep.Title = TruncTitle(messages[0].Content)
		}
		ep.Summary = result.Summary
		ep.Tags = result.Tags
	}

	ep.Close()
	return ep.Save()
}

// updateRunSummary refreshes the episode's running summary from recent messages.
func (m *Manager) updateRunSummary(ctx context.Context, ep *store.Episode) {
	messages, err := ep.TextMessages()
	if err != nil || len(messages) == 0 {
		return
	}

	const recentCap = 8
	tail := messages
	if len(tail) > recentCap {
		tail = tail[len(tail)-recentCap:]
	}

	var transcript strings.Builder
	for _, msg := range tail {
		fmt.Fprintf(&transcript, "[%s]: %s\n", msg.Role, msg.Content)
	}

	var prompt string
	if ep.RunSummary != "" {
		prompt = fmt.Sprintf(
			"Previous summary:\n%s\n\nNew messages:\n%s\n\nUpdate the summary in 2-3 sentences. Return ONLY the summary text, no JSON.",
			ep.RunSummary, transcript.String(),
		)
	} else {
		prompt = fmt.Sprintf(
			"Conversation so far:\n%s\n\nSummarize in 2-3 sentences. Return ONLY the summary text, no JSON.",
			transcript.String(),
		)
	}

	resp, err := m.llm.Call(ctx, []types.Message{
		types.NewUserMessage(types.TextBlock(prompt)),
	}, llm.CallOptions{
		System:    "You produce concise conversation summaries. Return only plain text, no JSON.",
		NoTools:   true,
		MaxTokens: runSummaryMaxTokens,
	})
	if err != nil {
		log.Warn().Err(err).Str("episode", ep.ID).Msg("episode: run summary update failed")
		return
	}

	ep.RunSummary = strings.TrimSpace(types.ExtractText(resp.Content))
	if err := ep.Save(); err != nil {
		log.Warn().Err(err).Str("episode", ep.ID).Msg("episode: failed to save run summary")
	}
}

// ShouldUpdateRunSummary returns true when the message count crosses a summary interval boundary.
func shouldUpdateRunSummary(count int) bool {
	return count > 0 && count%summaryInterval == 0
}

func parseSummaryJSON(text string) summaryResult {
	var result summaryResult
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		start := strings.Index(text, "{")
		end := strings.LastIndex(text, "}")
		if start >= 0 && end > start {
			_ = json.Unmarshal([]byte(text[start:end+1]), &result)
		}
	}
	return result
}
