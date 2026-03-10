package client

import (
	"context"
	"fmt"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/rs/zerolog/log"
)

type UpdateHandler func(ctx context.Context, update *models.Update)

type Client struct {
	b *bot.Bot
}

func New(token string, handler UpdateHandler) (*Client, error) {
	b, err := bot.New(token, bot.WithDefaultHandler(func(ctx context.Context, _ *bot.Bot, update *models.Update) {
		handler(ctx, update)
	}))
	if err != nil {
		return nil, fmt.Errorf("telegram bot init: %w", err)
	}
	log.Info().Msg("telegram bot authorized")
	return &Client{b: b}, nil
}

func (c *Client) Start(ctx context.Context) {
	c.b.Start(ctx)
}

func (c *Client) SetMyCommands(ctx context.Context, commands []models.BotCommand) {
	c.b.SetMyCommands(ctx, &bot.SetMyCommandsParams{Commands: commands})
}

// EscapeMarkdown escapes special characters for Telegram MarkdownV2.
func EscapeMarkdown(s string) string {
	return bot.EscapeMarkdown(s)
}
