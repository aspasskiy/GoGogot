package telegram

import (
	"context"
	"fmt"
	"gogogot/internal/channel/telegram/client"
	"gogogot/internal/core/transport"
	"strconv"
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

func (r *replier) sendStatus(ctx context.Context, status transport.AgentStatus) (string, error) {
	msgID := r.ch.sendAndGetID(ctx, r.chatID, formatStatus(status))
	return strconv.Itoa(msgID), nil
}

func (r *replier) updateStatus(ctx context.Context, statusID string, status transport.AgentStatus) error {
	msgID, err := strconv.Atoi(statusID)
	if err != nil {
		return fmt.Errorf("invalid status ID: %w", err)
	}
	if msgID == 0 {
		return nil
	}
	if err := r.ch.client.EditMessage(ctx, r.chatID, msgID, formatStatus(status), models.ParseModeMarkdown); err != nil {
		log.Warn().Err(err).Int("msg_id", msgID).Str("phase", string(status.Phase)).Msg("telegram: EditMessage failed")
	}
	return nil
}

func (r *replier) deleteStatus(ctx context.Context, statusID string) error {
	msgID, err := strconv.Atoi(statusID)
	if err != nil {
		return fmt.Errorf("invalid status ID: %w", err)
	}
	if msgID == 0 {
		return nil
	}
	if err := r.ch.client.DeleteMessage(ctx, r.chatID, msgID); err != nil {
		log.Warn().Err(err).Int("msg_id", msgID).Msg("telegram: DeleteMessage failed")
	}
	return nil
}
