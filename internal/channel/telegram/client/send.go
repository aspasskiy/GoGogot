package client

import (
	"context"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func (c *Client) SendMessage(ctx context.Context, chatID int64, text string, parseMode models.ParseMode) (int, error) {
	sent, err := c.b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    chatID,
		Text:      text,
		ParseMode: parseMode,
	})
	if err != nil {
		return 0, err
	}
	return sent.ID, nil
}

func (c *Client) SendPhoto(ctx context.Context, chatID int64, photo *models.InputFileUpload, caption string) error {
	_, err := c.b.SendPhoto(ctx, &bot.SendPhotoParams{
		ChatID: chatID, Photo: photo, Caption: caption,
	})
	return err
}

func (c *Client) SendVideo(ctx context.Context, chatID int64, video *models.InputFileUpload, caption string) error {
	_, err := c.b.SendVideo(ctx, &bot.SendVideoParams{
		ChatID: chatID, Video: video, Caption: caption,
	})
	return err
}

func (c *Client) SendAudio(ctx context.Context, chatID int64, audio *models.InputFileUpload, caption string) error {
	_, err := c.b.SendAudio(ctx, &bot.SendAudioParams{
		ChatID: chatID, Audio: audio, Caption: caption,
	})
	return err
}

func (c *Client) SendVoice(ctx context.Context, chatID int64, voice *models.InputFileUpload, caption string) error {
	_, err := c.b.SendVoice(ctx, &bot.SendVoiceParams{
		ChatID: chatID, Voice: voice, Caption: caption,
	})
	return err
}

func (c *Client) SendAnimation(ctx context.Context, chatID int64, animation *models.InputFileUpload, caption string) error {
	_, err := c.b.SendAnimation(ctx, &bot.SendAnimationParams{
		ChatID: chatID, Animation: animation, Caption: caption,
	})
	return err
}

func (c *Client) SendDocument(ctx context.Context, chatID int64, document *models.InputFileUpload, caption string) error {
	_, err := c.b.SendDocument(ctx, &bot.SendDocumentParams{
		ChatID: chatID, Document: document, Caption: caption,
	})
	return err
}

func (c *Client) SendTyping(ctx context.Context, chatID int64) error {
	_, err := c.b.SendChatAction(ctx, &bot.SendChatActionParams{
		ChatID: chatID,
		Action: models.ChatActionTyping,
	})
	return err
}
