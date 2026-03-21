package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"gogogot/internal/llm"
	"gogogot/internal/llm/types"
	"gogogot/internal/tools/store"
)

const maxSummariesForSearch = 50

const searchSystem = `You search through conversation history summaries.
You receive a numbered list of past conversation summaries and a query.
Return ONLY a JSON array of the numbers that are relevant to the query.
Example: [1, 4, 7]
If nothing is relevant, return: []
No explanation, no extra text.`

// SearchRelevant uses the LLM to find past chats semantically related to the query.
func (m *Manager) SearchRelevant(ctx context.Context, query string) ([]store.ChatInfo, error) {
	all, err := m.store.ListChats()
	if err != nil {
		return nil, err
	}

	var closed []store.ChatInfo
	for _, ch := range all {
		if ch.Status == "closed" && ch.Summary != "" {
			closed = append(closed, ch)
		}
	}
	if len(closed) == 0 {
		return nil, nil
	}
	if len(closed) > maxSummariesForSearch {
		closed = closed[:maxSummariesForSearch]
	}

	var catalog strings.Builder
	for i, ch := range closed {
		title := ch.Title
		if title == "" {
			title = "Untitled"
		}
		date := ch.StartedAt.Format("02 Jan 2006")
		fmt.Fprintf(&catalog, "%d. [%s] (%s): %s", i+1, title, date, ch.Summary)
		if len(ch.Tags) > 0 {
			fmt.Fprintf(&catalog, " [tags: %s]", strings.Join(ch.Tags, ", "))
		}
		catalog.WriteByte('\n')
	}

	prompt := fmt.Sprintf("Past conversations:\n%s\n---\nQuery: %s", catalog.String(), query)

	resp, err := m.llm.Call(ctx, []types.Message{
		types.NewUserMessage(types.TextBlock(prompt)),
	}, llm.CallOptions{
		System:  searchSystem,
		NoTools: true,
	})
	if err != nil {
		return nil, fmt.Errorf("search LLM call: %w", err)
	}

	indices := parseSearchResponse(types.ExtractText(resp.Content))

	var matches []store.ChatInfo
	for _, idx := range indices {
		if idx >= 1 && idx <= len(closed) {
			matches = append(matches, closed[idx-1])
		}
	}

	const maxResults = 5
	if len(matches) > maxResults {
		matches = matches[:maxResults]
	}

	return matches, nil
}

// SearchChats performs a simple word-based search over closed chats.
func (m *Manager) SearchChats(query string) ([]store.ChatInfo, error) {
	all, err := m.store.ListChats()
	if err != nil {
		return nil, err
	}

	q := strings.ToLower(query)
	words := strings.Fields(q)
	if len(words) == 0 {
		return nil, nil
	}

	var matches []store.ChatInfo
	for _, ch := range all {
		if ch.Status != "closed" || ch.Summary == "" {
			continue
		}
		corpus := strings.ToLower(ch.Title + " " + ch.Summary + " " + strings.Join(ch.Tags, " "))
		matched := false
		for _, w := range words {
			if strings.Contains(corpus, w) {
				matched = true
				break
			}
		}
		if matched {
			matches = append(matches, ch)
		}
	}

	const maxResults = 5
	if len(matches) > maxResults {
		matches = matches[:maxResults]
	}
	return matches, nil
}

func parseSearchResponse(text string) []int {
	text = strings.TrimSpace(text)

	var indices []int
	if err := json.Unmarshal([]byte(text), &indices); err != nil {
		start := strings.Index(text, "[")
		end := strings.LastIndex(text, "]")
		if start >= 0 && end > start {
			_ = json.Unmarshal([]byte(text[start:end+1]), &indices)
		}
	}
	return indices
}
