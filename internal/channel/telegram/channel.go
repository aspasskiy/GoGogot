package telegram

import (
	"context"
	"fmt"
	"gogogot/internal/channel"
	"gogogot/internal/channel/telegram/client"
	"strings"
	"sync"
	"time"

	"github.com/go-telegram/bot/models"
	"github.com/rs/zerolog/log"
)

type mediaGroupBuffer struct {
	messages []*models.Message
	timer    *time.Timer
}

type Channel struct {
	client  *client.Client
	ownerID int64

	handler channel.Handler

	mu          sync.Mutex
	mediaGroups map[string]*mediaGroupBuffer
}

func New(token string, ownerID int64) (*Channel, error) {
	t := &Channel{
		ownerID:     ownerID,
		mediaGroups: make(map[string]*mediaGroupBuffer),
	}

	c, err := client.New(token, t.defaultHandler)
	if err != nil {
		return nil, err
	}
	t.client = c

	return t, nil
}

type replier struct {
	ch     *Channel
	chatID int64
}

func (t *Channel) Name() string   { return "telegram" }
func (t *Channel) OwnerID() int64 { return t.ownerID }

func (t *Channel) OwnerSession() (string, channel.Replier) {
	return fmt.Sprintf("%s%d", channelPrefix, t.ownerID), t.newReplier(t.ownerID)
}

func (t *Channel) newReplier(chatID int64) *replier {
	return &replier{ch: t, chatID: chatID}
}

func (t *Channel) Run(ctx context.Context, handler channel.Handler) error {
	t.handler = handler

	t.client.SetMyCommands(ctx, []models.BotCommand{
		{Command: "new", Description: "Start a fresh conversation"},
		{Command: "history", Description: "View past conversation episodes"},
		{Command: "memory", Description: "List memory files"},
		{Command: "stop", Description: "Cancel the current task"},
		{Command: "help", Description: "Show available commands"},
	})

	log.Info().Int64("owner_id", t.ownerID).Msg("telegram bot polling started")
	t.client.Start(ctx)
	return ctx.Err()
}

func (t *Channel) defaultHandler(ctx context.Context, update *models.Update) {
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

func (t *Channel) handleMediaGroup(ctx context.Context, msg *models.Message) {
	t.mu.Lock()
	defer t.mu.Unlock()

	groupID := msg.MediaGroupID
	if buf, ok := t.mediaGroups[groupID]; ok {
		buf.messages = append(buf.messages, msg)
		buf.timer.Reset(1 * time.Second)
	} else {
		buf := &mediaGroupBuffer{
			messages: []*models.Message{msg},
		}
		buf.timer = time.AfterFunc(1*time.Second, func() {
			if ctx.Err() != nil {
				return
			}
			t.mu.Lock()
			msgs := t.mediaGroups[groupID].messages
			delete(t.mediaGroups, groupID)
			t.mu.Unlock()

			t.convertAndDispatch(ctx, msgs)
		})
		t.mediaGroups[groupID] = buf
	}
}

func basename(path string) string {
	i := strings.LastIndex(path, "/")
	if i < 0 {
		return path
	}
	return path[i+1:]
}
