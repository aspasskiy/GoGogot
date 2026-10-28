package transport

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aspasskiy/gogogot/internal/domain"
	"github.com/aspasskiy/gogogot/internal/utils"

	"github.com/rs/zerolog/log"
)

func ProcessAttachments(chatID, task string, attachments []Attachment) ([]domain.ContentBlock, func()) {
	if len(attachments) == 0 {
		return []domain.ContentBlock{domain.TextBlock(task)}, func() {}
	}

	tmpDir := filepath.Join(os.TempDir(), "gogogot-uploads",
		fmt.Sprintf("%s-%d", chatID, time.Now().UnixNano()))
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		log.Error().Err(err).Msg("failed to create upload dir")
		return []domain.ContentBlock{domain.TextBlock(task)}, func() {}
	}

	var imageBlocks []domain.ContentBlock
	var paths []string
	nameCount := map[string]int{}

	for _, att := range attachments {
		name := utils.UniqueName(att.Filename, nameCount)
		fpath := filepath.Join(tmpDir, name)
		if err := os.WriteFile(fpath, att.Data, 0644); err != nil {
			log.Error().Err(err).Str("path", fpath).Msg("failed to save attachment")
			continue
		}
		paths = append(paths, fpath)

		if strings.HasPrefix(att.MimeType, "image/") {
			b64 := base64.StdEncoding.EncodeToString(att.Data)
			imageBlocks = append(imageBlocks, domain.ImageBlock(att.MimeType, b64))
		}
	}

	pathList := strings.Join(paths, "\n- ")
	info := fmt.Sprintf("[Attached files saved to disk:\n- %s]", pathList)
	text := task
	if text != "" {
		text += "\n\n" + info
	} else {
		text = info
	}

	blocks := make([]domain.ContentBlock, 0, 1+len(imageBlocks))
	blocks = append(blocks, domain.TextBlock(text))
	blocks = append(blocks, imageBlocks...)

	cleanup := func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			log.Warn().Err(err).Str("dir", tmpDir).Msg("failed to remove upload dir")
		}
	}
	return blocks, cleanup
}
