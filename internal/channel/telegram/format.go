package telegram

import (
	"fmt"
	"gogogot/internal/tools/store"
	"strings"
)

func FormatHistory(episodes []store.EpisodeInfo) string {
	var closed []store.EpisodeInfo
	for _, ep := range episodes {
		if ep.Status == "closed" {
			closed = append(closed, ep)
		}
	}
	if len(closed) == 0 {
		return ""
	}

	const maxShown = 15
	if len(closed) > maxShown {
		closed = closed[:maxShown]
	}

	var sb strings.Builder
	sb.WriteString("📜 **Conversation history:**\n\n")
	for _, ep := range closed {
		title := ep.Title
		if title == "" {
			title = "Untitled"
		}
		if len([]rune(title)) > 50 {
			title = string([]rune(title)[:50]) + "…"
		}
		date := ep.StartedAt.Format("02 Jan")
		if !ep.EndedAt.IsZero() && ep.EndedAt.Format("02 Jan") != date {
			date += " — " + ep.EndedAt.Format("02 Jan")
		}
		fmt.Fprintf(&sb, "**%s** (%s)\n", title, date)
		if ep.Summary != "" {
			summary := ep.Summary
			if len([]rune(summary)) > 120 {
				summary = string([]rune(summary)[:120]) + "…"
			}
			fmt.Fprintf(&sb, "*%s*\n", summary)
		}
		if len(ep.Tags) > 0 {
			fmt.Fprintf(&sb, "`%s`\n", strings.Join(ep.Tags, ", "))
		}
		sb.WriteByte('\n')
	}
	return sb.String()
}

func FormatMemory(files []store.MemoryFile) string {
	if len(files) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("📂 **Memory files:**\n\n")
	for _, f := range files {
		fmt.Fprintf(&sb, "`%s` (%d bytes)\n", f.Name, f.Size)
	}
	return sb.String()
}
