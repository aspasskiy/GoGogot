package telegram

import (
	"bytes"
	"context"
	"fmt"
	"github.com/aspasskiy/gogogot/internal/core/transport"
	"github.com/aspasskiy/gogogot/internal/infra/utils"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-telegram/bot/models"
)

type replier struct {
	ch     *Channel
	chatID int64
}

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
	upload := &models.InputFileUpload{Filename: filepath.Base(path), Data: bytes.NewReader(data)}

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

func (r *replier) SendAsk(ctx context.Context, prompt string, kind transport.AskKind, options []transport.AskOption) error {
	text := "❓ " + prompt

	switch kind {
	case transport.AskConfirm:
		keyboard := [][]models.InlineKeyboardButton{{
			{Text: "✅ Yes", CallbackData: "yes"},
			{Text: "❌ No", CallbackData: "no"},
		}}
		_, err := r.ch.client.SendMessageWithKeyboard(ctx, r.chatID, text, "", keyboard)
		return err

	case transport.AskChoice:
		var rows [][]models.InlineKeyboardButton
		for _, opt := range options {
			rows = append(rows, []models.InlineKeyboardButton{
				{Text: opt.Label, CallbackData: opt.Value},
			})
		}
		_, err := r.ch.client.SendMessageWithKeyboard(ctx, r.chatID, text, "", rows)
		return err

	default:
		r.ch.sendLong(ctx, r.chatID, text)
		return nil
	}
}

func (r *replier) SendTyping(ctx context.Context) error {
	return r.ch.client.SendTyping(ctx, r.chatID)
}
