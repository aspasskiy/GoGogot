package telegram

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"gogogot/transport"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/rs/zerolog/log"
)

type mediaGroupBuffer struct {
	messages []*models.Message
	timer    *time.Timer
}

type ChatInfo struct {
	ID        string
	Title     string
	UpdatedAt time.Time
}

type MemoryFileInfo struct {
	Name string
	Size int64
}

type ChatLister interface {
	ListChats() ([]ChatInfo, error)
	GetExternalMapping(channelID string) (string, error)
}

type MemoryLister interface {
	ListMemory() ([]MemoryFileInfo, error)
}

type Transport struct {
	b       *bot.Bot
	ownerID int64

	chatLister   ChatLister
	memoryLister MemoryLister

	handler transport.Handler

	mu          sync.Mutex
	mediaGroups map[string]*mediaGroupBuffer
}

func New(token string, ownerID int64, cl ChatLister, ml MemoryLister) (*Transport, error) {
	t := &Transport{
		ownerID:      ownerID,
		chatLister:   cl,
		memoryLister: ml,
		mediaGroups:  make(map[string]*mediaGroupBuffer),
	}

	opts := []bot.Option{
		bot.WithDefaultHandler(t.defaultHandler),
		bot.WithCallbackQueryDataHandler(callbackPrefix, bot.MatchTypePrefix, t.callbackHandler),
	}

	b, err := bot.New(token, opts...)
	if err != nil {
		return nil, fmt.Errorf("telegram bot init: %w", err)
	}
	t.b = b

	log.Info().Msg("telegram bot authorized")
	return t, nil
}

func (t *Transport) Name() string { return "telegram" }

func (t *Transport) Run(ctx context.Context, handler transport.Handler) error {
	t.handler = handler

	t.b.SetMyCommands(ctx, &bot.SetMyCommandsParams{
		Commands: []models.BotCommand{
			{Command: "new", Description: "Start a fresh chat"},
			{Command: "chats", Description: "List and switch chats"},
			{Command: "memory", Description: "List memory files"},
			{Command: "stop", Description: "Cancel the current task"},
			{Command: "help", Description: "Show available commands"},
		},
	})

	log.Info().Int64("owner_id", t.ownerID).Msg("telegram bot polling started")
	t.b.Start(ctx)
	return ctx.Err()
}

func (t *Transport) defaultHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}
	msg := update.Message
	if msg.From == nil || msg.From.ID != t.ownerID {
		log.Debug().Msg("ignoring message from non-owner")
		return
	}

	if msg.MediaGroupID != "" {
		t.handleMediaGroup(ctx, msg)
	} else {
		t.convertAndDispatch(ctx, []*models.Message{msg})
	}
}

func (t *Transport) callbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	t.handleCallback(ctx, update.CallbackQuery)
}

// --- transport.Transport: SendText ---

func (t *Transport) SendText(ctx context.Context, channelID string, text string) error {
	chatID, err := parseChatID(channelID)
	if err != nil {
		return err
	}
	t.sendLong(ctx, chatID, text)
	return nil
}

// --- transport.FileSender ---

func (t *Transport) SendFile(ctx context.Context, channelID, path, caption string) error {
	chatID, err := parseChatID(channelID)
	if err != nil {
		return err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	lower := strings.ToLower(path)
	upload := &models.InputFileUpload{Filename: filepath(path), Data: bytes.NewReader(data)}

	switch {
	case hasAnySuffix(lower, ".jpg", ".jpeg", ".png", ".webp"):
		_, err = t.b.SendPhoto(ctx, &bot.SendPhotoParams{
			ChatID: chatID, Photo: upload, Caption: caption,
		})
	case hasAnySuffix(lower, ".mp4", ".mov", ".avi", ".mkv"):
		_, err = t.b.SendVideo(ctx, &bot.SendVideoParams{
			ChatID: chatID, Video: upload, Caption: caption,
		})
	case hasAnySuffix(lower, ".mp3", ".wav", ".flac", ".aac", ".m4a"):
		_, err = t.b.SendAudio(ctx, &bot.SendAudioParams{
			ChatID: chatID, Audio: upload, Caption: caption,
		})
	case hasAnySuffix(lower, ".ogg", ".opus"):
		_, err = t.b.SendVoice(ctx, &bot.SendVoiceParams{
			ChatID: chatID, Voice: upload, Caption: caption,
		})
	case hasAnySuffix(lower, ".gif"):
		_, err = t.b.SendAnimation(ctx, &bot.SendAnimationParams{
			ChatID: chatID, Animation: upload, Caption: caption,
		})
	default:
		_, err = t.b.SendDocument(ctx, &bot.SendDocumentParams{
			ChatID: chatID, Document: upload, Caption: caption,
		})
	}
	return err
}

// --- transport.TypingNotifier ---

func (t *Transport) SendTyping(ctx context.Context, channelID string) error {
	chatID, err := parseChatID(channelID)
	if err != nil {
		return err
	}
	_, err = t.b.SendChatAction(ctx, &bot.SendChatActionParams{
		ChatID: chatID,
		Action: models.ChatActionTyping,
	})
	return err
}

// --- transport.StatusUpdater ---

var phaseEmoji = map[transport.Phase]string{
	transport.PhaseThinking: "\U0001f9e0",
	transport.PhasePlanning: "\U0001f4cb",
	transport.PhaseTool:     "\U0001f527",
}

func formatStatus(s transport.AgentStatus) string {
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
	return emoji + " " + bot.EscapeMarkdown(label) + "\\.\\.\\."
}

func (t *Transport) SendStatus(ctx context.Context, channelID string, status transport.AgentStatus) (string, error) {
	chatID, err := parseChatID(channelID)
	if err != nil {
		return "", err
	}
	msgID := t.sendAndGetID(ctx, chatID, formatStatus(status))
	return strconv.Itoa(msgID), nil
}

func (t *Transport) UpdateStatus(ctx context.Context, channelID, statusID string, status transport.AgentStatus) error {
	chatID, err := parseChatID(channelID)
	if err != nil {
		return err
	}
	msgID, err := strconv.Atoi(statusID)
	if err != nil {
		return fmt.Errorf("invalid status ID: %w", err)
	}
	t.editMessage(ctx, chatID, msgID, formatStatus(status))
	return nil
}

func (t *Transport) DeleteStatus(ctx context.Context, channelID, statusID string) error {
	chatID, err := parseChatID(channelID)
	if err != nil {
		return err
	}
	msgID, err := strconv.Atoi(statusID)
	if err != nil {
		return fmt.Errorf("invalid status ID: %w", err)
	}
	t.deleteMessage(ctx, chatID, msgID)
	return nil
}

// --- API accessor ---

func (t *Transport) OwnerID() int64 { return t.ownerID }

// --- internal helpers ---

func parseChatID(channelID string) (int64, error) {
	return strconv.ParseInt(strings.TrimPrefix(channelID, channelPrefix), 10, 64)
}

func hasAnySuffix(s string, suffixes ...string) bool {
	for _, suf := range suffixes {
		if strings.HasSuffix(s, suf) {
			return true
		}
	}
	return false
}

func filepath(path string) string {
	i := strings.LastIndex(path, "/")
	if i < 0 {
		return path
	}
	return path[i+1:]
}
