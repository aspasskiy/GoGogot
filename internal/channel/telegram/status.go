package telegram

import (
	"context"
	"fmt"
	"gogogot/internal/channel"
	"gogogot/internal/channel/telegram/client"
	"strconv"

	"github.com/go-telegram/bot/models"
	"github.com/rs/zerolog/log"
)

var phaseEmoji = map[channel.Phase]string{
	channel.PhaseThinking: "\U0001f9e0",
	channel.PhasePlanning: "\U0001f4cb",
	channel.PhaseTool:     "\U0001f527",
}

func formatStatus(s channel.AgentStatus) string {
	emoji := phaseEmoji[s.Phase]
	if emoji == "" {
		emoji = "\u23f3"
	}
	label := s.Detail
	if label == "" {
		switch s.Phase {
		case channel.PhaseThinking:
			label = "Thinking"
		case channel.PhasePlanning:
			label = "Planning"
		default:
			label = s.Tool
		}
	}
	return emoji + " " + client.EscapeMarkdown(label) + "\\.\\.\\."
}

func (r *replier) SendStatus(ctx context.Context, status channel.AgentStatus) (string, error) {
	msgID := r.ch.sendAndGetID(ctx, r.chatID, formatStatus(status))
	return strconv.Itoa(msgID), nil
}

func (r *replier) UpdateStatus(ctx context.Context, statusID string, status channel.AgentStatus) error {
	msgID, err := strconv.Atoi(statusID)
	if err != nil {
		return fmt.Errorf("invalid status ID: %w", err)
	}
	if msgID == 0 {
		return nil
	}
	err = r.ch.client.EditMessage(ctx, r.chatID, msgID, formatStatus(status), models.ParseModeMarkdown)
	if err != nil {
		log.Debug().Err(err).Msg("telegram edit failed")
	}
	return nil
}

func (r *replier) DeleteStatus(ctx context.Context, statusID string) error {
	msgID, err := strconv.Atoi(statusID)
	if err != nil {
		return fmt.Errorf("invalid status ID: %w", err)
	}
	if msgID == 0 {
		return nil
	}
	err = r.ch.client.DeleteMessage(ctx, r.chatID, msgID)
	if err != nil {
		log.Debug().Err(err).Msg("telegram delete failed")
	}
	return nil
}
