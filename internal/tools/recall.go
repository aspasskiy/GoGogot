package tools

import (
	"context"
	"fmt"
	"github.com/aspasskiy/gogogot/internal/tools/store"
	"github.com/aspasskiy/gogogot/internal/tools/types"
	"strings"
)

// RecallTool builds the recall tool that delegates search to the provided function.
func RecallTool(searchFn store.EpisodeSearchFunc) types.Tool {
	return types.Tool{
		Name:  "recall",
		Label: "Recalling history",
		Description: "Search your conversation history for past context. Use when the user references something from a previous conversation, or when you need to recall what was discussed before. Returns summaries of matching past episodes.",
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
			for i, ep := range matches {
				if i > 0 {
					sb.WriteString("\n---\n")
				}
				dateRange := ep.StartedAt.Format("02 Jan 2006")
				if !ep.EndedAt.IsZero() && ep.EndedAt.Format("02 Jan 2006") != dateRange {
					dateRange += " — " + ep.EndedAt.Format("02 Jan 2006")
				}
				title := ep.Title
				if title == "" {
					title = "Untitled"
				}
				fmt.Fprintf(&sb, "[Episode: %s (%s)]\n%s", title, dateRange, ep.Summary)
				if len(ep.Tags) > 0 {
					fmt.Fprintf(&sb, "\nTags: %s", strings.Join(ep.Tags, ", "))
				}
				sb.WriteByte('\n')
			}

			return types.Result{Output: sb.String()}
		},
	}
}
