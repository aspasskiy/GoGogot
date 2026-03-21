package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"gogogot/internal/infra/utils"
	"gogogot/internal/llm"
	"gogogot/internal/llm/types"
	"gogogot/internal/tools/store"

	"github.com/rs/zerolog/log"
)

const (
	summaryMaxTokens    = 300
	summarizeInterval   = 5
	firstSummarizeAfter = 1
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

// SummarizeIfNeeded runs background summarization after the first user message
// and then every summarizeInterval user messages.
func (m *Manager) SummarizeIfNeeded(ctx context.Context, ch *store.Chat) {
	n := ch.UserTurns
	if n < firstSummarizeAfter {
		return
	}
	if n != firstSummarizeAfter && n%summarizeInterval != 0 {
		return
	}

	m.bgMu.Lock()
	if m.bgRunning[ch.ID] {
		m.bgMu.Unlock()
		return
	}
	m.bgRunning[ch.ID] = true
	m.bgMu.Unlock()

	go func() {
		defer func() {
			m.bgMu.Lock()
			delete(m.bgRunning, ch.ID)
			m.bgMu.Unlock()
		}()
		m.summarize(ctx, ch)
	}()
}

// Close marks the chat closed. If the summary is already populated
// (from background summarization) it skips the LLM call.
func (m *Manager) Close(ctx context.Context, ch *store.Chat) error {
	if ch.Summary == "" {
		m.summarize(ctx, ch)
	}
	if ch.Title == "" {
		if msgs, err := ch.TextMessages(); err == nil && len(msgs) > 0 {
			ch.Title = TruncTitle(msgs[0].Content)
		}
	}
	ch.Close()
	return ch.Save()
}

func (m *Manager) summarize(ctx context.Context, ch *store.Chat) {
	messages, err := ch.TextMessages()
	if err != nil || len(messages) == 0 {
		return
	}

	var transcript strings.Builder
	for _, msg := range messages {
		fmt.Fprintf(&transcript, "[%s]: %s\n", msg.Role, msg.Content)
	}

	prompt := "Summarize this conversation. Return ONLY valid JSON:\n" +
		`{"title": "short title", "summary": "2-3 sentence summary", "tags": ["tag1", "tag2"]}` +
		"\n\nPreserve: key decisions, outcomes, important facts, action items.\n\n---\n\n" +
		transcript.String()

	resp, err := m.llm.Call(ctx, []types.Message{
		types.NewUserMessage(types.TextBlock(prompt)),
	}, llm.CallOptions{
		System:    "You summarize conversations into structured JSON. Be concise and accurate.",
		NoTools:   true,
		MaxTokens: summaryMaxTokens,
	})

	if err != nil {
		log.Error().Err(err).Str("chat", ch.ID).Msg("chat: summarization failed")
		if ch.Title == "" && len(messages) > 0 {
			ch.Title = TruncTitle(messages[0].Content)
		}
		return
	}

	text := types.ExtractText(resp.Content)
	result := parseSummaryJSON(text)
	if result.Title != "" {
		ch.Title = result.Title
	} else if ch.Title == "" && len(messages) > 0 {
		ch.Title = TruncTitle(messages[0].Content)
	}
	ch.Summary = result.Summary
	ch.Tags = result.Tags

	if err := ch.Save(); err != nil {
		log.Error().Err(err).Str("chat", ch.ID).Msg("chat: failed to save after summarization")
	}
	log.Info().Str("chat", ch.ID).Int("user_turns", ch.UserTurns).Str("title", ch.Title).Msg("chat: summarized")
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

// bgGuard is embedded in Manager to track in-flight background summarizations.
type bgGuard struct {
	bgMu      sync.Mutex
	bgRunning map[string]bool
}
