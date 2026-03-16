package telegram

import (
	"context"
	"fmt"
	"github.com/aspasskiy/gogogot/internal/channel/telegram/client"
	"github.com/aspasskiy/gogogot/internal/channel/telegram/format"
	"github.com/aspasskiy/gogogot/internal/core/transport"
	"strings"

	"github.com/go-telegram/bot/models"
	"github.com/rs/zerolog/log"
)

var phaseEmoji = map[transport.Phase]string{
	transport.PhaseThinking: "\U0001f9e0",
	transport.PhasePlanning: "\U0001f4cb",
	transport.PhaseTool:     "\U0001f527",
	transport.PhaseWorking:  "\u26a1",
	transport.PhaseMessage:  "\U0001f4ac",
}

func formatStatus(s transport.AgentStatus) string {
	var parts []string

	if len(s.Plan) > 0 {
		parts = append(parts, formatPlanLine(s.Plan))
	}

	if s.Percent != nil {
		parts = append(parts, formatProgressBar(*s.Percent))
	}

	emoji := phaseEmoji[s.Phase]
	if emoji == "" {
		emoji = "\u23f3"
	}

	label := s.Detail
	if label == "" {
		switch s.Phase {
		case transport.PhaseThinking:
			label = "Thinking"
		case transport.PhasePlanning:
			label = "Planning"
		default:
			label = s.Tool
		}
	}
	if label != "" {
		if s.Phase == transport.PhaseMessage {
			runes := []rune(label)
			if len(runes) > 300 {
				label = string(runes[:300]) + "…"
			}
			parts = append(parts, emoji+" "+client.EscapeMarkdown(label))
		} else {
			parts = append(parts, emoji+" "+client.EscapeMarkdown(label)+"\\.\\.\\.")
		}
	}

	if len(parts) == 0 {
		return emoji + " Working\\.\\.\\."
	}
	return strings.Join(parts, "\n")
}

func formatProgressBar(pct int) string {
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	const width = 10
	filled := pct * width / 100
	bar := strings.Repeat("\u2588", filled) + strings.Repeat("\u2591", width-filled)
	return fmt.Sprintf("`%s` %d%%", bar, pct)
}

func formatPlanLine(tasks []transport.PlanTask) string {
	if len(tasks) == 0 {
		return ""
	}
	completed := 0
	var activeTitle string
	var icons []string
	for _, t := range tasks {
		switch t.Status {
		case transport.TaskCompleted:
			completed++
			icons = append(icons, "\u2705")
		case transport.TaskInProgress:
			icons = append(icons, "\u25b8")
			if activeTitle == "" {
				activeTitle = t.Title
			}
		default:
			icons = append(icons, "\u25cb")
		}
	}

	line := fmt.Sprintf("\U0001f4cb %d/%d %s", completed, len(tasks), strings.Join(icons, ""))
	if activeTitle != "" {
		line += "\n\u25b8 " + client.EscapeMarkdown(activeTitle)
	}
	return line
}

func (r *replier) sendStatus(ctx context.Context, status transport.AgentStatus) int {
	return r.ch.sendAndGetID(ctx, r.chatID, formatStatus(status))
}

func (r *replier) updateStatus(ctx context.Context, msgID int, status transport.AgentStatus) {
	if msgID == 0 {
		return
	}
	if err := r.ch.client.EditMessage(ctx, r.chatID, msgID, formatStatus(status), models.ParseModeMarkdown); err != nil {
		log.Warn().Err(err).Int("msg_id", msgID).Str("phase", string(status.Phase)).Msg("telegram: EditMessage failed")
	}
}

func (r *replier) deleteStatus(ctx context.Context, msgID int) {
	if msgID == 0 {
		return
	}
	if err := r.ch.client.DeleteMessage(ctx, r.chatID, msgID); err != nil {
		log.Warn().Err(err).Int("msg_id", msgID).Msg("telegram: DeleteMessage failed")
	}
}

// editToFinal replaces the status message with the final response text.
// If the text fits in one message, it edits in-place. Otherwise it deletes
// the status and sends the full text as chunked HTML messages.
func (r *replier) editToFinal(ctx context.Context, msgID int, text string) {
	if msgID == 0 {
		r.ch.sendLong(ctx, r.chatID, text)
		return
	}

	chunks := format.FormatHTMLChunks(text, maxMessageLen)
	if len(chunks) == 0 {
		r.deleteStatus(ctx, msgID)
		return
	}

	// Single chunk: edit the status message in-place.
	if len(chunks) == 1 {
		err := r.ch.client.EditMessage(ctx, r.chatID, msgID, chunks[0].HTML, models.ParseModeHTML)
		if err != nil {
			log.Warn().Err(err).Msg("telegram: edit to final HTML failed, trying plain text")
			err = r.ch.client.EditMessage(ctx, r.chatID, msgID, chunks[0].Text, "")
			if err != nil {
				log.Warn().Err(err).Msg("telegram: edit to final plain failed, falling back to delete+send")
				r.deleteStatus(ctx, msgID)
				r.ch.sendLong(ctx, r.chatID, text)
			}
		}
		return
	}

	// Multiple chunks: edit status with first chunk, send the rest as new messages.
	err := r.ch.client.EditMessage(ctx, r.chatID, msgID, chunks[0].HTML, models.ParseModeHTML)
	if err != nil {
		log.Warn().Err(err).Msg("telegram: edit first chunk failed, falling back to delete+send")
		r.deleteStatus(ctx, msgID)
		r.ch.sendLong(ctx, r.chatID, text)
		return
	}
	for _, chunk := range chunks[1:] {
		r.ch.sendHTMLChunk(ctx, r.chatID, chunk)
	}
}
