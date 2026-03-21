package tools

import (
	"context"
	"fmt"
	"gogogot/internal/tools/store"
	"gogogot/internal/tools/types"
	"strings"
)

// RecallTool builds the recall tool that delegates search to the provided function.
func RecallTool(searchFn store.ChatSearchFunc) types.Tool {
	return types.Tool{
		Name:  "recall",
		Label: "Recalling history",
		Description: "Search your conversation history for past context. Use when the user references something from a previous conversation, or when you need to recall what was discussed before. Returns summaries of matching past conversations.",
		Parameters: map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "What to search for — topic, keyword, or question about past conversations",
			},
		},
		Required: []string{"query"},
		Handler: func(ctx context.Context, input map[string]any) types.Result {
			query, err := types.GetString(input, "query")
			if err != nil {
				return types.ErrResult(err)
			}

			matches, err := searchFn(ctx, query)
			if err != nil {
				return types.Result{Output: "error searching history: " + err.Error(), IsErr: true}
			}

			if len(matches) == 0 {
				return types.Result{Output: "No relevant past conversations found."}
			}

			var sb strings.Builder
			for i, ch := range matches {
				if i > 0 {
					sb.WriteString("\n---\n")
				}
				dateRange := ch.StartedAt.Format("02 Jan 2006")
				if !ch.EndedAt.IsZero() && ch.EndedAt.Format("02 Jan 2006") != dateRange {
					dateRange += " — " + ch.EndedAt.Format("02 Jan 2006")
				}
				title := ch.Title
				if title == "" {
					title = "Untitled"
				}
				fmt.Fprintf(&sb, "[Chat: %s (%s)]\n%s", title, dateRange, ch.Summary)
				if len(ch.Tags) > 0 {
					fmt.Fprintf(&sb, "\nTags: %s", strings.Join(ch.Tags, ", "))
				}
				sb.WriteByte('\n')
			}

			return types.Result{Output: sb.String()}
		},
	}
}
