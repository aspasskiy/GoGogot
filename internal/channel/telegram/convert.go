package telegram

import (
	"context"
	"fmt"
	"gogogot/internal/channel"
	"strings"

	"github.com/go-telegram/bot/models"
	"github.com/rs/zerolog/log"
)

type mediaExtractor struct {
	check   func(*models.Message) bool
	process func(t *Channel, ctx context.Context, msg *models.Message) ([]channel.Attachment, error)
}

var mediaExtractors = []mediaExtractor{
	{
		check: func(m *models.Message) bool { return m.Animation != nil },
		process: func(t *Channel, ctx context.Context, m *models.Message) ([]channel.Attachment, error) {
			return t.processAnimation(ctx, m.Animation)
		},
	},
	{
		check: func(m *models.Message) bool { return m.Document != nil && m.Animation == nil },
		process: func(t *Channel, ctx context.Context, m *models.Message) ([]channel.Attachment, error) {
			return t.processDocument(ctx, m.Document)
		},
	},
	{
		check: func(m *models.Message) bool { return len(m.Photo) > 0 },
		process: func(t *Channel, ctx context.Context, m *models.Message) ([]channel.Attachment, error) {
			return t.processPhoto(ctx, m.Photo)
		},
	},
	{
		check: func(m *models.Message) bool { return m.Audio != nil },
		process: func(t *Channel, ctx context.Context, m *models.Message) ([]channel.Attachment, error) {
			return t.processAudio(ctx, m.Audio)
		},
	},
	{
		check: func(m *models.Message) bool { return m.Voice != nil },
		process: func(t *Channel, ctx context.Context, m *models.Message) ([]channel.Attachment, error) {
			return t.processVoice(ctx, m.Voice)
		},
	},
	{
		check: func(m *models.Message) bool { return m.Video != nil },
		process: func(t *Channel, ctx context.Context, m *models.Message) ([]channel.Attachment, error) {
			return t.processVideo(ctx, m.Video)
		},
	},
	{
		check: func(m *models.Message) bool { return m.VideoNote != nil },
		process: func(t *Channel, ctx context.Context, m *models.Message) ([]channel.Attachment, error) {
			return t.processVideoNote(ctx, m.VideoNote)
		},
	},
	{
		check: func(m *models.Message) bool { return m.Sticker != nil },
		process: func(t *Channel, ctx context.Context, m *models.Message) ([]channel.Attachment, error) {
			return t.processSticker(ctx, m.Sticker)
		},
	},
}

func (t *Channel) convertAndDispatch(ctx context.Context, msgs []*models.Message) {
	if len(msgs) == 0 {
		return
	}

	chatID := msgs[0].Chat.ID
	channelID := fmt.Sprintf("%s%d", channelPrefix, chatID)
	var textParts []string
	var attachments []channel.Attachment

	for _, msg := range msgs {
		text := strings.TrimSpace(msg.Text)
		if text == "" {
			text = strings.TrimSpace(msg.Caption)
		}
		if text != "" {
			textParts = append(textParts, text)
		}

		for _, ex := range mediaExtractors {
			if ex.check(msg) {
				atts, err := ex.process(t, ctx, msg)
				if err != nil {
					log.Error().Err(err).Msg("failed to process media")
				} else {
					attachments = append(attachments, atts...)
				}
			}
		}

		if msg.Venue != nil {
			textParts = append(textParts, fmt.Sprintf("[Venue: %s, %s — lat=%.6f, lon=%.6f]",
				msg.Venue.Title, msg.Venue.Address,
				msg.Venue.Location.Latitude, msg.Venue.Location.Longitude))
		} else if msg.Location != nil {
			textParts = append(textParts, fmt.Sprintf("[Location: lat=%.6f, lon=%.6f]",
				msg.Location.Latitude, msg.Location.Longitude))
		}

		if msg.Contact != nil {
			textParts = append(textParts, fmt.Sprintf("[Contact: %s %s, phone: %s]",
				msg.Contact.FirstName, msg.Contact.LastName, msg.Contact.PhoneNumber))
		}

		if msg.Poll != nil {
			opts := make([]string, len(msg.Poll.Options))
			for i, o := range msg.Poll.Options {
				opts[i] = o.Text
			}
			textParts = append(textParts, fmt.Sprintf("[Poll: %s — options: %s]",
				msg.Poll.Question, strings.Join(opts, ", ")))
		}

		if msg.Dice != nil {
			textParts = append(textParts, fmt.Sprintf("[Dice: %s = %d]",
				msg.Dice.Emoji, msg.Dice.Value))
		}
	}

	text := strings.Join(textParts, "\n\n")

	var fileTexts []string
	for _, att := range attachments {
		if !strings.HasPrefix(att.MimeType, "image/") && isTextMIME(att.MimeType) {
			fileTexts = append(fileTexts, fmt.Sprintf("[File: %s]\n```\n%s\n```", att.Filename, string(att.Data)))
		}
	}

	if len(fileTexts) > 0 {
		filesStr := strings.Join(fileTexts, "\n\n")
		if text != "" {
			text = filesStr + "\n\n" + text
		} else {
			text = filesStr
		}
	}

	if text == "" && len(attachments) == 0 {
		return
	}

	if text == "" && len(attachments) > 0 {
		text = "What's in these files?"
	}

	log.Debug().
		Int64("chat_id", chatID).
		Int("text_len", len(text)).
		Int("attachments", len(attachments)).
		Msg("telegram incoming message")

	if strings.HasPrefix(text, "/") {
		cmdName := strings.Fields(text)[0]
		log.Info().Str("cmd", cmdName).Msg("command received")
		t.handleCommand(ctx, chatID, channelID, cmdName)
		return
	}

	t.handler(ctx, channel.Message{
		ChannelID:   channelID,
		Text:        text,
		Attachments: attachments,
	})
}
