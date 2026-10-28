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

// ExpandAttachments prepares a received message for the agent: zip and tar.gz
// archives are replaced by the text and image files inside them, text files are
// inlined into the message, and a message with only files gets a default
// question. Channels call it before handing the message to the engine.
func ExpandAttachments(text string, attachments []Attachment) (string, []Attachment) {
	text = strings.TrimSpace(text)

	var expanded []Attachment
	for _, att := range attachments {
		if !IsArchive(att.MimeType, att.Filename) {
			expanded = append(expanded, att)
			continue
		}
		var (
			files []Attachment
			err   error
		)
		if isArchiveZip(att.MimeType, att.Filename) {
			files, err = extractZipFiles(att.Data)
		} else {
			files, err = extractTarGzFiles(att.Data)
		}
		if err != nil {
			log.Error().Err(err).Str("file", att.Filename).Msg("failed to unpack archive")
			continue
		}
		expanded = append(expanded, files...)
	}

	var fileTexts []string
	for _, att := range expanded {
		if !strings.HasPrefix(att.MimeType, "image/") && IsTextMIME(att.MimeType) {
			fileTexts = append(fileTexts, fmt.Sprintf("[File: %s]\n```\n%s\n```", att.Filename, string(att.Data)))
		}
	}
	if len(fileTexts) > 0 {
		filesStr := strings.Join(fileTexts, "\n\n")
		if text != "" {
			text = filesStr + "\n\n" + text
		} else {
			text = filesStr
		}
	}

	if text == "" && len(expanded) > 0 {
		text = "What's in these files?"
	}
	return text, expanded
}

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
