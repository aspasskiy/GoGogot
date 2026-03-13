package telegram

import (
	"context"
	"fmt"
	"gogogot/internal/channel"
	"gogogot/internal/channel/telegram/client"
	"gogogot/internal/core/transport"
	"gogogot/internal/tools/store"
	"strings"
)

var commandMap = map[string]string{
	"/start":   channel.CmdNewEpisode,
	"/new":     channel.CmdNewEpisode,
	"/stop":    channel.CmdStop,
	"/history": channel.CmdHistory,
	"/memory":  channel.CmdMemory,
}

var commandSuccess = map[string]string{
	channel.CmdNewEpisode: "✨ New conversation started.",
}

var commandEmpty = map[string]string{
	channel.CmdHistory: "No conversation history yet.",
	channel.CmdMemory:  "Memory is empty — no files yet.",
}

func (c *Channel) handleCommand(ctx context.Context, chatID int64, reply transport.Replier, cmdText string) {
	if cmdText == "/help" {
		c.send(ctx, chatID, "*Commands:*\n"+
			"/new — start a fresh conversation\n"+
			"/history — view past conversation episodes\n"+
			"/memory — list memory files\n"+
			"/stop — cancel the current task\n"+
			"/help — show this help")
		return
	}

	name, ok := commandMap[cmdText]
	if !ok {
		c.send(ctx, chatID, "Unknown command\\. Try /help")
		return
	}

	cmd := &channel.Command{Name: name, Result: &channel.CommandResult{}}
	c.handler(ctx, channel.Message{Reply: reply, Command: cmd})

	if cmd.Result.Error != nil {
		c.send(ctx, chatID, "Error: "+client.EscapeMarkdown(cmd.Result.Error.Error()))
		return
	}

	if text := formatPayload(cmd.Result.Payload); text != "" {
		c.sendLong(ctx, chatID, text)
		return
	}

	if text := cmd.Result.Data["text"]; text != "" {
		c.sendLong(ctx, chatID, text)
		return
	}

	if msg, ok := commandSuccess[name]; ok {
		c.sendLong(ctx, chatID, msg)
		return
	}

	if msg, ok := commandEmpty[name]; ok {
		c.sendLong(ctx, chatID, msg)
	}
}

func formatPayload(payload any) string {
	switch v := payload.(type) {
	case []store.EpisodeInfo:
		return FormatHistory(v)
	case []store.MemoryFile:
		return FormatMemory(v)
	default:
		return ""
	}
}

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
