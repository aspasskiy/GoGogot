package telegram

import (
	"context"
	"github.com/aspasskiy/gogogot/internal/channel"
	"time"

	"github.com/go-telegram/bot/models"
	"github.com/rs/zerolog/log"
)

type mediaGroupBuffer struct {
	messages []*models.Message
	timer    *time.Timer
}

func (c *Channel) defaultHandler(ctx context.Context, update *models.Update) {
	if update.CallbackQuery != nil {
		c.handleCallback(ctx, update.CallbackQuery)
		return
	}

	if update.Message == nil {
		return
	}
	msg := update.Message
	if msg.From == nil || msg.From.ID != c.ownerID {
		log.Trace().Msg("ignoring message from non-owner")
		return
	}

	if msg.MediaGroupID != "" {
		c.handleMediaGroup(ctx, msg)
	} else {
		c.convertAndDispatch(ctx, []*models.Message{msg})
	}
}

func (c *Channel) handleCallback(ctx context.Context, cb *models.CallbackQuery) {
	if cb.From.ID != c.ownerID {
		return
	}
	_ = c.client.AnswerCallbackQuery(ctx, cb.ID)

	var chatID int64
	if cb.Message.Message != nil {
		chatID = cb.Message.Message.Chat.ID
	} else {
		chatID = cb.From.ID
	}
	c.handler(ctx, channel.Message{
		Text:  cb.Data,
		Reply: c.newReplier(chatID),
	})
}

func (c *Channel) handleMediaGroup(ctx context.Context, msg *models.Message) {
	c.mu.Lock()
	defer c.mu.Unlock()

	groupID := msg.MediaGroupID
	if buf, ok := c.mediaGroups[groupID]; ok {
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
			c.mu.Lock()
			msgs := c.mediaGroups[groupID].messages
			delete(c.mediaGroups, groupID)
			c.mu.Unlock()

			c.convertAndDispatch(ctx, msgs)
		})
		c.mediaGroups[groupID] = buf
	}
}
