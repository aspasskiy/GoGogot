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

func (r *replier) SendText(ctx context.Context, text string) error {
	r.ch.sendLong(ctx, r.chatID, text)
	return nil
}

func (r *replier) SendFile(ctx context.Context, path, caption string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	lower := strings.ToLower(path)
	upload := &models.InputFileUpload{Filename: basename(path), Data: bytes.NewReader(data)}

	switch {
	case utils.HasAnySuffix(lower, ".jpg", ".jpeg", ".png", ".webp"):
		return r.ch.client.SendPhoto(ctx, r.chatID, upload, caption)
	case utils.HasAnySuffix(lower, ".mp4", ".mov", ".avi", ".mkv"):
		return r.ch.client.SendVideo(ctx, r.chatID, upload, caption)
	case utils.HasAnySuffix(lower, ".mp3", ".wav", ".flac", ".aac", ".m4a"):
		return r.ch.client.SendAudio(ctx, r.chatID, upload, caption)
	case utils.HasAnySuffix(lower, ".ogg", ".opus"):
		return r.ch.client.SendVoice(ctx, r.chatID, upload, caption)
	case utils.HasAnySuffix(lower, ".gif"):
		return r.ch.client.SendAnimation(ctx, r.chatID, upload, caption)
	default:
		return r.ch.client.SendDocument(ctx, r.chatID, upload, caption)
	}
}

func (r *replier) SendTyping(ctx context.Context) error {
	return r.ch.client.SendTyping(ctx, r.chatID)
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
