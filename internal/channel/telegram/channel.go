package telegram

import (
	"context"
	"github.com/aspasskiy/gogogot/internal/channel"
	"github.com/aspasskiy/gogogot/internal/channel/telegram/client"
	"github.com/aspasskiy/gogogot/internal/core/transport"
	"sync"

	"github.com/go-telegram/bot/models"
	"github.com/rs/zerolog/log"
)

type Channel struct {
	client  *client.Client
	ownerID int64

	handler channel.Handler

	mu          sync.Mutex
	mediaGroups map[string]*mediaGroupBuffer
}

func New(token string, ownerID int64) (*Channel, error) {
	ch := &Channel{
		ownerID:     ownerID,
		mediaGroups: make(map[string]*mediaGroupBuffer),
	}

	cl, err := client.New(token, ch.defaultHandler)
	if err != nil {
		return nil, err
	}
	ch.client = cl

	return ch, nil
}

func (c *Channel) Name() string   { return "telegram" }
func (c *Channel) OwnerID() int64 { return c.ownerID }

func (c *Channel) OwnerReplier() transport.Replier {
	return c.newReplier(c.ownerID)
}

func (c *Channel) newReplier(chatID int64) *replier {
	return &replier{ch: c, chatID: chatID}
}

func (c *Channel) Run(ctx context.Context, handler channel.Handler) error {
	c.handler = handler

	c.client.SetMyCommands(ctx, []models.BotCommand{
		{Command: "new", Description: "Start a fresh conversation"},
		{Command: "history", Description: "View past conversation episodes"},
		{Command: "memory", Description: "List memory files"},
		{Command: "stop", Description: "Cancel the current task"},
		{Command: "help", Description: "Show available commands"},
	})

	log.Info().Int64("owner_id", c.ownerID).Msg("telegram bot polling started")
	c.client.Start(ctx)
	return ctx.Err()
}
