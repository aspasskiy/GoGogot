package telegram

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"gogogot/infra/transport"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/rs/zerolog/log"
)

type mediaGroupBuffer struct {
	messages []*tgbotapi.Message
	timer    *time.Timer
}

type Transport struct {
	api     *tgbotapi.BotAPI
	ownerID int64

	mu          sync.Mutex
	mediaGroups map[string]*mediaGroupBuffer
}

func New(token string, ownerID int64) (*Transport, error) {
	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, fmt.Errorf("telegram bot init: %w", err)
	}
	log.Info().Str("username", api.Self.UserName).Msg("telegram bot authorized")
	return &Transport{
		api:         api,
		ownerID:     ownerID,
		mediaGroups: make(map[string]*mediaGroupBuffer),
	}, nil
}

func (t *Transport) Name() string { return "telegram" }

func (t *Transport) Run(ctx context.Context, handler transport.Handler) error {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 30
	updates := t.api.GetUpdatesChan(u)

	log.Info().Int64("owner_id", t.ownerID).Msg("telegram bot polling started")
	for {
		select {
		case <-ctx.Done():
			t.api.StopReceivingUpdates()
			return ctx.Err()
		case update := <-updates:
			if update.CallbackQuery != nil {
				t.handleCallback(ctx, update.CallbackQuery, handler)
				continue
			}
			if update.Message == nil {
				continue
			}
			if update.Message.From.ID != t.ownerID {
				log.Debug().Int64("user_id", update.Message.From.ID).Msg("ignoring message from non-owner")
				continue
			}

			if update.Message.MediaGroupID != "" {
				t.handleMediaGroup(ctx, update.Message, handler)
			} else {
				t.convertAndDispatch(ctx, []*tgbotapi.Message{update.Message}, handler)
			}
		}
	}
}

// --- transport.Transport: SendText ---

func (t *Transport) SendText(_ context.Context, channelID string, text string) error {
	chatID, err := t.parseChatID(channelID)
	if err != nil {
		return err
	}
	t.sendLong(chatID, text)
	return nil
}

// --- transport.FileSender ---

func (t *Transport) SendFile(_ context.Context, channelID, path, caption string) error {
	chatID, err := t.parseChatID(channelID)
	if err != nil {
		return err
	}

	lower := strings.ToLower(path)
	var msg tgbotapi.Chattable

	switch {
	case hasAnySuffix(lower, ".jpg", ".jpeg", ".png", ".webp"):
		p := tgbotapi.NewPhoto(chatID, tgbotapi.FilePath(path))
		if caption != "" {
			p.Caption = caption
		}
		msg = p
	case hasAnySuffix(lower, ".mp4", ".mov", ".avi", ".mkv"):
		v := tgbotapi.NewVideo(chatID, tgbotapi.FilePath(path))
		if caption != "" {
			v.Caption = caption
		}
		msg = v
	case hasAnySuffix(lower, ".mp3", ".wav", ".flac", ".aac", ".m4a"):
		a := tgbotapi.NewAudio(chatID, tgbotapi.FilePath(path))
		if caption != "" {
			a.Caption = caption
		}
		msg = a
	case hasAnySuffix(lower, ".ogg", ".opus"):
		vc := tgbotapi.NewVoice(chatID, tgbotapi.FilePath(path))
		if caption != "" {
			vc.Caption = caption
		}
		msg = vc
	case hasAnySuffix(lower, ".gif"):
		an := tgbotapi.NewAnimation(chatID, tgbotapi.FilePath(path))
		if caption != "" {
			an.Caption = caption
		}
		msg = an
	default:
		d := tgbotapi.NewDocument(chatID, tgbotapi.FilePath(path))
		if caption != "" {
			d.Caption = caption
		}
		msg = d
	}

	_, sendErr := t.api.Send(msg)
	return sendErr
}

// --- transport.TypingNotifier ---

func (t *Transport) SendTyping(_ context.Context, channelID string) error {
	chatID, err := t.parseChatID(channelID)
	if err != nil {
		return err
	}
	action := tgbotapi.NewChatAction(chatID, tgbotapi.ChatTyping)
	_, err = t.api.Request(action)
	return err
}

// --- transport.StatusUpdater ---

func (t *Transport) SendStatus(_ context.Context, channelID, text string) (string, error) {
	chatID, err := t.parseChatID(channelID)
	if err != nil {
		return "", err
	}
	escaped := escapeMarkdown(text)
	msgID := t.sendAndGetID(chatID, escaped)
	return strconv.Itoa(msgID), nil
}

func (t *Transport) UpdateStatus(_ context.Context, channelID, statusID, text string) error {
	chatID, err := t.parseChatID(channelID)
	if err != nil {
		return err
	}
	msgID, err := strconv.Atoi(statusID)
	if err != nil {
		return fmt.Errorf("invalid status ID: %w", err)
	}
	t.editMessage(chatID, msgID, escapeMarkdown(text))
	return nil
}

func (t *Transport) DeleteStatus(_ context.Context, channelID, statusID string) error {
	chatID, err := t.parseChatID(channelID)
	if err != nil {
		return err
	}
	msgID, err := strconv.Atoi(statusID)
	if err != nil {
		return fmt.Errorf("invalid status ID: %w", err)
	}
	t.deleteMessage(chatID, msgID)
	return nil
}

// --- API accessor for Telegram-specific command handling ---

func (t *Transport) API() *tgbotapi.BotAPI { return t.api }
func (t *Transport) OwnerID() int64         { return t.ownerID }

// --- internal helpers ---

func (t *Transport) parseChatID(channelID string) (int64, error) {
	return strconv.ParseInt(strings.TrimPrefix(channelID, "tg_"), 10, 64)
}

func hasAnySuffix(s string, suffixes ...string) bool {
	for _, suf := range suffixes {
		if strings.HasSuffix(s, suf) {
			return true
		}
	}
	return false
}
