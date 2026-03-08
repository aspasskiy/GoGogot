package bridge

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gogogot/llm/types"
	"gogogot/transport"

	"github.com/rs/zerolog/log"
)

func processAttachments(chatID, task string, attachments []transport.Attachment) ([]types.ContentBlock, func()) {
	if len(attachments) == 0 {
		return []types.ContentBlock{types.TextBlock(task)}, func() {}
	}

	tmpDir := filepath.Join(os.TempDir(), "gogogot-uploads",
		fmt.Sprintf("%s-%d", chatID, time.Now().UnixNano()))
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		log.Error().Err(err).Msg("failed to create upload dir")
		return []types.ContentBlock{types.TextBlock(task)}, func() {}
	}

	var imageBlocks []types.ContentBlock
	var paths []string
	nameCount := map[string]int{}

	for _, att := range attachments {
		name := uniqueName(att.Filename, nameCount)
		fpath := filepath.Join(tmpDir, name)
		if err := os.WriteFile(fpath, att.Data, 0644); err != nil {
			log.Error().Err(err).Str("path", fpath).Msg("failed to save attachment")
			continue
		}
		paths = append(paths, fpath)

		if strings.HasPrefix(att.MimeType, "image/") {
			b64 := base64.StdEncoding.EncodeToString(att.Data)
			imageBlocks = append(imageBlocks, types.ImageBlock(att.MimeType, b64))
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

	blocks := make([]types.ContentBlock, 0, 1+len(imageBlocks))
	blocks = append(blocks, types.TextBlock(text))
	blocks = append(blocks, imageBlocks...)

	cleanup := func() { os.RemoveAll(tmpDir) }
	return blocks, cleanup
}

func uniqueName(name string, counts map[string]int) string {
	counts[name]++
	if counts[name] == 1 {
		return name
	}
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	return fmt.Sprintf("%s_%d%s", base, counts[name], ext)
}
