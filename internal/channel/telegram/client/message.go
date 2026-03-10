package client

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func (c *Client) EditMessage(ctx context.Context, chatID int64, messageID int, text string, parseMode models.ParseMode) error {
	_, err := c.b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:    chatID,
		MessageID: messageID,
		Text:      text,
		ParseMode: parseMode,
	})
	return err
}

func (c *Client) DeleteMessage(ctx context.Context, chatID int64, messageID int) error {
	_, err := c.b.DeleteMessage(ctx, &bot.DeleteMessageParams{
		ChatID:    chatID,
		MessageID: messageID,
	})
	return err
}

func (c *Client) DownloadFile(ctx context.Context, fileID string) ([]byte, error) {
	file, err := c.b.GetFile(ctx, &bot.GetFileParams{FileID: fileID})
	if err != nil {
		return nil, fmt.Errorf("get file info: %w", err)
	}
	url := c.b.FileDownloadLink(file)
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download: HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}
