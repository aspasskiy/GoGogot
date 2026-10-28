package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/aspasskiy/gogogot/internal/domain"
	"github.com/aspasskiy/gogogot/internal/llm"
	"github.com/aspasskiy/gogogot/internal/store"
	"github.com/aspasskiy/gogogot/internal/utils"

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
	n := ch.UserTurnCount()
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
	if ch.Info().Summary == "" {
		m.summarize(ctx, ch)
	}
	fallback := fallbackTitle(ch)
	return ch.Update(func(c *store.Chat) {
		if c.Title == "" {
			c.Title = fallback
		}
		c.Status = "closed"
		c.EndedAt = time.Now()
	})
}

// fallbackTitle derives a title from the first text message of the chat.
func fallbackTitle(ch *store.Chat) string {
	msgs, err := ch.TextMessages()
	if err != nil || len(msgs) == 0 {
		return ""
	}
	return TruncTitle(msgs[0].Content)
}

func (m *Manager) summarize(ctx context.Context, ch *store.Chat) {
	messages, err := ch.TextMessages()
	if err != nil || len(messages) == 0 {
		return
	}
	fallback := TruncTitle(messages[0].Content)

	var transcript strings.Builder
	for _, msg := range messages {
		fmt.Fprintf(&transcript, "[%s]: %s\n", msg.Role, msg.Content)
	}

	prompt := "Summarize this conversation. Return ONLY valid JSON:\n" +
		`{"title": "short title", "summary": "2-3 sentence summary", "tags": ["tag1", "tag2"]}` +
		"\n\nPreserve: key decisions, outcomes, important facts, action items.\n\n---\n\n" +
		transcript.String()

	resp, err := m.llm.Call(ctx, []domain.Message{
		domain.NewUserMessage(domain.TextBlock(prompt)),
	}, llm.CallOptions{
		System:    "You summarize conversations into structured JSON. Be concise and accurate.",
		NoTools:   true,
		MaxTokens: summaryMaxTokens,
	})

	if err != nil {
		log.Error().Err(err).Str("chat", ch.ID).Msg("chat: summarization failed")
		if err := ch.Update(func(c *store.Chat) {
			if c.Title == "" {
				c.Title = fallback
			}
		}); err != nil {
			log.Error().Err(err).Str("chat", ch.ID).Msg("chat: failed to save fallback title")
		}
		return
	}

	result := parseSummaryJSON(domain.ExtractText(resp.Content))
	if err := ch.Update(func(c *store.Chat) {
		switch {
		case result.Title != "":
			c.Title = result.Title
		case c.Title == "":
			c.Title = fallback
		}
		c.Summary = result.Summary
		c.Tags = result.Tags
	}); err != nil {
		log.Error().Err(err).Str("chat", ch.ID).Msg("chat: failed to save after summarization")
	}

	info := ch.Info()
	log.Info().Str("chat", ch.ID).Int("user_turns", ch.UserTurnCount()).Str("title", info.Title).Msg("chat: summarized")
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
