package telegram

import (
	"fmt"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/rs/zerolog/log"
)

func (t *Transport) send(chatID int64, markdownText string) {
	msg := tgbotapi.NewMessage(chatID, markdownText)
	msg.ParseMode = tgbotapi.ModeMarkdownV2
	if _, err := t.api.Send(msg); err != nil {
		log.Error().Err(err).Msg("telegram send failed")
	}
}

func (t *Transport) sendAndGetID(chatID int64, markdownText string) int {
	msg := tgbotapi.NewMessage(chatID, markdownText)
	msg.ParseMode = tgbotapi.ModeMarkdownV2
	sent, err := t.api.Send(msg)
	if err != nil {
		log.Error().Err(err).Msg("telegram send failed")
		return 0
	}
	return sent.MessageID
}

func (t *Transport) sendLong(chatID int64, text string) {
	const maxLen = 4000
	for len(text) > 0 {
		chunk := text
		if len(chunk) > maxLen {
			cut := strings.LastIndex(chunk[:maxLen], "\n")
			if cut < maxLen/2 {
				cut = maxLen
			}
			chunk = text[:cut]
			text = text[cut:]
		} else {
			text = ""
		}

		msg := tgbotapi.NewMessage(chatID, chunk)
		msg.ParseMode = tgbotapi.ModeMarkdown
		if _, err := t.api.Send(msg); err != nil {
			log.Warn().Err(err).Msg("telegram markdown send failed, falling back to plain text")
			msg.ParseMode = ""
			if _, err := t.api.Send(msg); err != nil {
				log.Error().Err(err).Msg("telegram plain text send failed")
			}
		}
	}
}

func (t *Transport) editMessage(chatID int64, messageID int, markdownText string) {
	if messageID == 0 {
		return
	}
	edit := tgbotapi.NewEditMessageText(chatID, messageID, markdownText)
	edit.ParseMode = tgbotapi.ModeMarkdownV2
	if _, err := t.api.Send(edit); err != nil {
		log.Debug().Err(err).Msg("telegram edit failed")
	}
}

func (t *Transport) deleteMessage(chatID int64, messageID int) {
	if messageID == 0 {
		return
	}
	del := tgbotapi.NewDeleteMessage(chatID, messageID)
	if _, err := t.api.Request(del); err != nil {
		log.Debug().Err(err).Msg("telegram delete failed")
	}
}

// SendMessage sends a one-shot message to a Telegram chat (used by --task mode).
func SendMessage(token string, chatID int64, text string) error {
	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return fmt.Errorf("telegram init: %w", err)
	}
	const maxLen = 4000
	for len(text) > 0 {
		chunk := text
		if len(chunk) > maxLen {
			cut := strings.LastIndex(chunk[:maxLen], "\n")
			if cut < maxLen/2 {
				cut = maxLen
			}
			chunk = text[:cut]
			text = text[cut:]
		} else {
			text = ""
		}

		msg := tgbotapi.NewMessage(chatID, chunk)
		msg.ParseMode = tgbotapi.ModeMarkdown
		if _, err := api.Send(msg); err != nil {
			msg.ParseMode = ""
			if _, err := api.Send(msg); err != nil {
				return fmt.Errorf("telegram send: %w", err)
			}
		}
	}
	return nil
}


func escapeMarkdown(s string) string {
	replacer := strings.NewReplacer(
		"_", "\\_", "*", "\\*", "[", "\\[", "]", "\\]",
		"(", "\\(", ")", "\\)", "~", "\\~", "`", "\\`",
		">", "\\>", "#", "\\#", "+", "\\+", "-", "\\-",
		"=", "\\=", "|", "\\|", "{", "\\{", "}", "\\}",
		".", "\\.", "!", "\\!",
	)
	return replacer.Replace(s)
}
