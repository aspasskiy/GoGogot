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
	"/start":   channel.CmdNewChat,
	"/new":     channel.CmdNewChat,
	"/stop":    channel.CmdStop,
	"/history": channel.CmdHistory,
	"/memory":  channel.CmdMemory,
	"/soul":    channel.CmdSoul,
	"/user":    channel.CmdUser,
}

var commandSuccess = map[string]string{
	channel.CmdNewChat: "✨ New conversation started.",
}

var commandEmpty = map[string]string{
	channel.CmdHistory: "No conversation history yet.",
	channel.CmdMemory:  "Memory is empty — no files yet.",
	channel.CmdSoul:    "Soul not configured yet — no soul.md.",
	channel.CmdUser:    "User profile not configured yet — no user.md.",
}

func (c *Channel) handleCommand(ctx context.Context, chatID int64, reply transport.Replier, cmdText string) {
	if cmdText == "/help" {
		c.send(ctx, chatID, "*Commands:*\n"+
			"/new — start a fresh conversation\n"+
			"/history — view past conversations\n"+
			"/memory — list memory files\n"+
			"/soul — show agent identity\n"+
			"/user — show user profile\n"+
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
	case []store.ChatInfo:
		return FormatHistory(v)
	case []store.MemoryFile:
		return FormatMemory(v)
	default:
		return ""
	}
}

func FormatHistory(chats []store.ChatInfo) string {
	var closed []store.ChatInfo
	for _, ch := range chats {
		if ch.Status == "closed" {
			closed = append(closed, ch)
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
	for _, ch := range closed {
		title := ch.Title
		if title == "" {
			title = "Untitled"
		}
		if len([]rune(title)) > 50 {
			title = string([]rune(title)[:50]) + "…"
		}
		date := ch.StartedAt.Format("02 Jan")
		if !ch.EndedAt.IsZero() && ch.EndedAt.Format("02 Jan") != date {
			date += " — " + ch.EndedAt.Format("02 Jan")
		}
		fmt.Fprintf(&sb, "**%s** (%s)\n", title, date)
		if ch.Summary != "" {
			summary := ch.Summary
			if len([]rune(summary)) > 120 {
				summary = string([]rune(summary)[:120]) + "…"
			}
			fmt.Fprintf(&sb, "*%s*\n", summary)
		}
		if len(ch.Tags) > 0 {
			fmt.Fprintf(&sb, "`%s`\n", strings.Join(ch.Tags, ", "))
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
	for i, f := range files {
		if i > 0 {
			sb.WriteString("\n---\n\n")
		}
		fmt.Fprintf(&sb, "📂 **%s**\n\n%s\n", f.Name, strings.TrimSpace(f.Content))
	}
	return sb.String()
}

