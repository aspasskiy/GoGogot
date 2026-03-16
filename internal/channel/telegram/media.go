package telegram

import (
	"context"
	"fmt"
	"github.com/aspasskiy/gogogot/internal/core/transport"

	"github.com/go-telegram/bot/models"
)

func (c *Channel) downloadMedia(ctx context.Context, fileID string, fileSize, maxSize int64, filename, mime string) ([]transport.Attachment, error) {
	if fileSize > maxSize {
		return nil, fmt.Errorf("file too large (%d bytes)", fileSize)
	}
	data, err := c.client.DownloadFile(ctx, fileID)
	if err != nil {
		return nil, err
	}
	return []transport.Attachment{{Filename: filename, MimeType: mime, Data: data}}, nil
}

func (c *Channel) processDocument(ctx context.Context, doc *models.Document) ([]transport.Attachment, error) {
	mime := doc.MimeType

	if isArchiveZip(mime, doc.FileName) {
		if doc.FileSize > maxGenericFileSize {
			return nil, fmt.Errorf("zip file too large (%d bytes)", doc.FileSize)
		}
		data, err := c.client.DownloadFile(ctx, doc.FileID)
		if err != nil {
			return nil, err
		}
		return extractZipFiles(data)
	}

	if isArchiveTarGz(mime, doc.FileName) {
		if doc.FileSize > maxGenericFileSize {
			return nil, fmt.Errorf("tar.gz file too large (%d bytes)", doc.FileSize)
		}
		data, err := c.client.DownloadFile(ctx, doc.FileID)
		if err != nil {
			return nil, err
		}
		return extractTarGzFiles(data)
	}

	if isImageMIME(mime) {
		return c.downloadMedia(ctx, doc.FileID, doc.FileSize, maxImageFileSize, doc.FileName, mime)
	}

	if isTextMIME(mime) || mime == "" || mime == "application/octet-stream" {
		return c.downloadMedia(ctx, doc.FileID, doc.FileSize, maxTextFileSize, doc.FileName, "text/plain")
	}

	return c.downloadMedia(ctx, doc.FileID, doc.FileSize, maxGenericFileSize, doc.FileName, mime)
}

func (c *Channel) processPhoto(ctx context.Context, photos []models.PhotoSize) ([]transport.Attachment, error) {
	if len(photos) == 0 {
		return nil, nil
	}
	largest := photos[len(photos)-1]
	return c.downloadMedia(ctx, largest.FileID, int64(largest.FileSize), maxImageFileSize, "photo.jpg", "image/jpeg")
}

func (c *Channel) processAudio(ctx context.Context, audio *models.Audio) ([]transport.Attachment, error) {
	filename := audio.FileName
	if filename == "" {
		filename = "audio.mp3"
	}
	mime := audio.MimeType
	if mime == "" {
		mime = "audio/mpeg"
	}
	return c.downloadMedia(ctx, audio.FileID, audio.FileSize, maxGenericFileSize, filename, mime)
}

func (c *Channel) processVoice(ctx context.Context, voice *models.Voice) ([]transport.Attachment, error) {
	mime := voice.MimeType
	if mime == "" {
		mime = "audio/ogg"
	}
	return c.downloadMedia(ctx, voice.FileID, voice.FileSize, maxGenericFileSize, "voice.ogg", mime)
}

func (c *Channel) processVideo(ctx context.Context, video *models.Video) ([]transport.Attachment, error) {
	mime := video.MimeType
	if mime == "" {
		mime = "video/mp4"
	}
	filename := video.FileName
	if filename == "" {
		filename = "video.mp4"
	}
	return c.downloadMedia(ctx, video.FileID, video.FileSize, maxGenericFileSize, filename, mime)
}

func (c *Channel) processVideoNote(ctx context.Context, vn *models.VideoNote) ([]transport.Attachment, error) {
	return c.downloadMedia(ctx, vn.FileID, int64(vn.FileSize), maxGenericFileSize, "videonote.mp4", "video/mp4")
}

func (c *Channel) processAnimation(ctx context.Context, anim *models.Animation) ([]transport.Attachment, error) {
	mime := anim.MimeType
	if mime == "" {
		mime = "video/mp4"
	}
	filename := anim.FileName
	if filename == "" {
		filename = "animation.mp4"
	}
	return c.downloadMedia(ctx, anim.FileID, anim.FileSize, maxGenericFileSize, filename, mime)
}

func (c *Channel) processSticker(ctx context.Context, sticker *models.Sticker) ([]transport.Attachment, error) {
	if sticker.IsAnimated {
		return nil, nil
	}
	return c.downloadMedia(ctx, sticker.FileID, int64(sticker.FileSize), maxImageFileSize, "sticker.webp", "image/webp")
}
