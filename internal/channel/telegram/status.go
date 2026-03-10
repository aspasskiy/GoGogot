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

func (t *Channel) SendStatus(ctx context.Context, channelID string, status channel.AgentStatus) (string, error) {
	chatID, err := parseChatID(channelID)
	if err != nil {
		return "", err
	}
	msgID := t.sendAndGetID(ctx, chatID, formatStatus(status))
	return strconv.Itoa(msgID), nil
}

func (t *Channel) UpdateStatus(ctx context.Context, channelID, statusID string, status channel.AgentStatus) error {
	chatID, err := parseChatID(channelID)
	if err != nil {
		return err
	}
	msgID, err := strconv.Atoi(statusID)
	if err != nil {
		return fmt.Errorf("invalid status ID: %w", err)
	}
	if msgID == 0 {
		return nil
	}
	err = t.client.EditMessage(ctx, chatID, msgID, formatStatus(status), models.ParseModeMarkdown)
	if err != nil {
		log.Debug().Err(err).Msg("telegram edit failed")
	}
	return nil
}

func (t *Channel) DeleteStatus(ctx context.Context, channelID, statusID string) error {
	chatID, err := parseChatID(channelID)
	if err != nil {
		return err
	}
	msgID, err := strconv.Atoi(statusID)
	if err != nil {
		return fmt.Errorf("invalid status ID: %w", err)
	}
	if msgID == 0 {
		return nil
	}
	err = t.client.DeleteMessage(ctx, chatID, msgID)
	if err != nil {
		log.Debug().Err(err).Msg("telegram delete failed")
	}
	return nil
}
