package render

import (
	"context"
	"fmt"
	"strings"

	"github.com/aspasskiy/gogogot/internal/channel"
	"github.com/aspasskiy/gogogot/internal/domain"
	"github.com/aspasskiy/gogogot/internal/store"
	"github.com/aspasskiy/gogogot/internal/transport"

	"github.com/rs/zerolog/log"
)

// Inbound is one user message as a channel received it, before routing.
type Inbound struct {
	Chat        string
	Text        string
	Attachments []transport.Attachment
}

// Dispatch routes an inbound message: archives are unpacked and text files
// inlined, slash commands are executed and answered here, and everything else
// goes to the engine's handler with a replier for the chat.
func Dispatch(ctx context.Context, h channel.Handler, s Surface, in Inbound) {
	text, attachments := transport.ExpandAttachments(in.Text, in.Attachments)
	if text == "" && len(attachments) == 0 {
		return
	}

	reply := NewReplier(s, in.Chat)
	if strings.HasPrefix(text, "/") {
		cmd := strings.Fields(text)[0]
		if at := strings.IndexByte(cmd, '@'); at > 0 {
			cmd = cmd[:at] // "/new@my_bot" in group chats
		}
		log.Info().Str("cmd", cmd).Msg("command received")
		RunCommand(ctx, h, reply, cmd)
		return
	}

	h(ctx, channel.Message{
		Text:        text,
		Attachments: attachments,
		Reply:       reply,
	})
}

// Commands lists the slash commands every channel supports, for help texts
// and platform command menus.
var Commands = []struct{ Name, Description string }{
	{"new", "Start a fresh conversation"},
	{"history", "View past conversations"},
	{"memory", "List memory files"},
	{"soul", "Show agent identity (soul.md)"},
	{"user", "Show user profile (user.md)"},
	{"stop", "Cancel the current task"},
	{"help", "Show available commands"},
}

// HelpText is the reply to /help.
func HelpText() string {
	var b strings.Builder
	b.WriteString("**Commands:**\n")
	for _, c := range Commands {
		fmt.Fprintf(&b, "/%s — %s\n", c.Name, strings.ToLower(c.Description[:1])+c.Description[1:])
	}
	return b.String()
}

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

// RunCommand executes one slash command (e.g. "/new") and sends the answer.
func RunCommand(ctx context.Context, h channel.Handler, reply transport.Replier, cmdText string) {
	if cmdText == "/help" {
		_ = reply.SendText(ctx, HelpText())
		return
	}

	name, ok := commandMap[cmdText]
	if !ok {
		_ = reply.SendText(ctx, "Unknown command. Try /help")
		return
	}

	cmd := &channel.Command{Name: name, Result: &channel.CommandResult{}}
	h(ctx, channel.Message{Reply: reply, Command: cmd})

	if cmd.Result.Error != nil {
		_ = reply.SendText(ctx, "Error: "+cmd.Result.Error.Error())
		return
	}

	if text := formatPayload(cmd.Result.Payload); text != "" {
		_ = reply.SendText(ctx, text)
		return
	}

	if text := cmd.Result.Data["text"]; text != "" {
		_ = reply.SendText(ctx, text)
		return
	}

	if msg, ok := commandSuccess[name]; ok {
		_ = reply.SendText(ctx, msg)
		return
	}

	if msg, ok := commandEmpty[name]; ok {
		_ = reply.SendText(ctx, msg)
	}
}

func formatPayload(payload any) string {
	switch v := payload.(type) {
	case []domain.ChatInfo:
		return FormatHistory(v)
	case []store.MemoryFile:
		return FormatMemory(v)
	default:
		return ""
	}
}

func FormatHistory(chats []domain.ChatInfo) string {
	var closed []domain.ChatInfo
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
