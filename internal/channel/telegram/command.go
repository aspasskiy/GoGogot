package telegram

import (
	"context"
	"gogogot/internal/channel"
	"gogogot/internal/channel/telegram/client"
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

func (t *Channel) handleCommand(ctx context.Context, chatID int64, sessionID string, reply channel.Replier, cmdText string) {
	if cmdText == "/help" {
		t.send(ctx, chatID, "*Commands:*\n"+
			"/new — start a fresh conversation\n"+
			"/history — view past conversation episodes\n"+
			"/memory — list memory files\n"+
			"/stop — cancel the current task\n"+
			"/help — show this help")
		return
	}

	name, ok := commandMap[cmdText]
	if !ok {
		t.send(ctx, chatID, "Unknown command\\. Try /help")
		return
	}

	cmd := &channel.Command{Name: name, Result: &channel.CommandResult{}}
	t.handler(ctx, channel.Message{SessionID: sessionID, Reply: reply, Command: cmd})

	if cmd.Result.Error != nil {
		t.send(ctx, chatID, "Error: "+client.EscapeMarkdown(cmd.Result.Error.Error()))
		return
	}

	if text := cmd.Result.Data["text"]; text != "" {
		t.sendLong(ctx, chatID, text)
		return
	}

	if msg, ok := commandSuccess[name]; ok {
		t.sendLong(ctx, chatID, msg)
		return
	}

	if msg, ok := commandEmpty[name]; ok {
		t.sendLong(ctx, chatID, msg)
	}
}
