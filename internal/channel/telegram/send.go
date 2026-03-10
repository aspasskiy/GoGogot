package telegram

import (
	"bytes"
	"context"
	"fmt"
	"gogogot/internal/channel/telegram/format"
	"gogogot/internal/infra/utils"
	"os"
	"strings"

	"github.com/go-telegram/bot/models"
	"github.com/rs/zerolog/log"
)

func (t *Channel) SendText(ctx context.Context, channelID string, text string) error {
	chatID, err := parseChatID(channelID)
	if err != nil {
		return err
	}
	t.sendLong(ctx, chatID, text)
	return nil
}

func (t *Channel) SendFile(ctx context.Context, channelID, path, caption string) error {
	chatID, err := parseChatID(channelID)
	if err != nil {
		return err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	lower := strings.ToLower(path)
	upload := &models.InputFileUpload{Filename: basename(path), Data: bytes.NewReader(data)}

	switch {
	case utils.HasAnySuffix(lower, ".jpg", ".jpeg", ".png", ".webp"):
		return t.client.SendPhoto(ctx, chatID, upload, caption)
	case utils.HasAnySuffix(lower, ".mp4", ".mov", ".avi", ".mkv"):
		return t.client.SendVideo(ctx, chatID, upload, caption)
	case utils.HasAnySuffix(lower, ".mp3", ".wav", ".flac", ".aac", ".m4a"):
		return t.client.SendAudio(ctx, chatID, upload, caption)
	case utils.HasAnySuffix(lower, ".ogg", ".opus"):
		return t.client.SendVoice(ctx, chatID, upload, caption)
	case utils.HasAnySuffix(lower, ".gif"):
		return t.client.SendAnimation(ctx, chatID, upload, caption)
	default:
		return t.client.SendDocument(ctx, chatID, upload, caption)
	}
}

func (t *Channel) SendTyping(ctx context.Context, channelID string) error {
	chatID, err := parseChatID(channelID)
	if err != nil {
		return err
	}
	return t.client.SendTyping(ctx, chatID)
}

func (t *Channel) send(ctx context.Context, chatID int64, markdownText string) {
	_, err := t.client.SendMessage(ctx, chatID, markdownText, models.ParseModeMarkdown)
	if err != nil {
		log.Error().Err(err).Msg("telegram send failed")
	}
}

func (t *Channel) sendAndGetID(ctx context.Context, chatID int64, markdownText string) int {
	msgID, err := t.client.SendMessage(ctx, chatID, markdownText, models.ParseModeMarkdown)
	if err != nil {
		log.Error().Err(err).Msg("telegram send failed")
		return 0
	}
	return msgID
}

func (t *Channel) sendLong(ctx context.Context, chatID int64, text string) {
	for _, chunk := range format.FormatHTMLChunks(text, maxMessageLen) {
		_, err := t.client.SendMessage(ctx, chatID, chunk.HTML, models.ParseModeHTML)
		if err != nil {
			log.Warn().Err(err).Msg("telegram HTML send failed, falling back to plain text")
			_, err = t.client.SendMessage(ctx, chatID, chunk.Text, "")
			if err != nil {
				log.Error().Err(err).Msg("telegram plain text send failed")
			}
		}
	}
}
