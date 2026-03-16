package telegram

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"fmt"
	"github.com/aspasskiy/gogogot/internal/core/transport"
	"io"
	"path/filepath"
	"strings"
)

func isArchiveZip(mime, filename string) bool {
	return mime == "application/zip" ||
		mime == "application/x-zip-compressed" ||
		strings.HasSuffix(strings.ToLower(filename), ".zip")
}

func isArchiveTarGz(mime, filename string) bool {
	lower := strings.ToLower(filename)
	return mime == "application/gzip" ||
		mime == "application/x-gzip" ||
		strings.HasSuffix(lower, ".tar.gz") ||
		strings.HasSuffix(lower, ".tgz")
}

func isImageMIME(mime string) bool {
	return strings.HasPrefix(mime, "image/")
}

var textMIMEs = map[string]bool{
	"application/json":       true,
	"application/xml":        true,
	"application/javascript": true,
	"application/x-yaml":     true,
	"application/toml":       true,
	"application/x-sh":       true,
	"application/csv":        true,
	"application/sql":        true,
}

func isTextMIME(mime string) bool {
	return strings.HasPrefix(mime, "text/") || textMIMEs[mime]
}

func shouldIncludeArchiveEntry(name string) (isText, isImage bool) {
	if strings.HasPrefix(name, ".") || strings.Contains(name, "/.") || strings.Contains(name, "__MACOSX") {
		return false, false
	}
	lower := strings.ToLower(name)
	return isTextExtension(lower), isImageExtension(lower)
}

func extractZipFiles(data []byte) ([]transport.Attachment, error) {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("failed to read zip: %w", err)
	}

	var attachments []transport.Attachment
	for _, file := range reader.File {
		if file.FileInfo().IsDir() {
			continue
		}
		isText, isImage := shouldIncludeArchiveEntry(file.Name)
		if !isText && !isImage {
			continue
		}
		if isText && file.UncompressedSize64 > maxTextFileSize {
			continue
		}
		if isImage && file.UncompressedSize64 > maxImageFileSize {
			continue
		}

		rc, err := file.Open()
		if err != nil {
			continue
		}
		content, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			continue
		}

		attachments = append(attachments, transport.Attachment{
			Filename: file.Name,
			MimeType: mimeFromExtension(strings.ToLower(file.Name), isImage),
			Data:     content,
		})
		if len(attachments) >= maxArchiveEntries {
			break
		}
	}

	return attachments, nil
}

func extractTarGzFiles(data []byte) ([]transport.Attachment, error) {
	gzr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to read gzip: %w", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	var attachments []transport.Attachment

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read tar: %w", err)
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}

		isText, isImage := shouldIncludeArchiveEntry(header.Name)
		if !isText && !isImage {
			continue
		}
		if isText && header.Size > maxTextFileSize {
			continue
		}
		if isImage && header.Size > maxImageFileSize {
			continue
		}

		content, err := io.ReadAll(tr)
		if err != nil {
			continue
		}

		attachments = append(attachments, transport.Attachment{
			Filename: header.Name,
			MimeType: mimeFromExtension(strings.ToLower(header.Name), isImage),
			Data:     content,
		})
		if len(attachments) >= maxArchiveEntries {
			break
		}
	}

	return attachments, nil
}

var textExtensions = map[string]bool{
	".txt": true, ".md": true, ".json": true, ".go": true,
	".py": true, ".js": true, ".html": true, ".css": true,
	".csv": true, ".yaml": true, ".yml": true, ".xml": true,
}

func isTextExtension(name string) bool {
	return textExtensions[filepath.Ext(name)]
}

var imageExtensions = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".webp": true,
}

func isImageExtension(name string) bool {
	return imageExtensions[filepath.Ext(name)]
}

func mimeFromExtension(name string, isImage bool) string {
	if !isImage {
		return "text/plain"
	}
	if strings.HasSuffix(name, ".png") {
		return "image/png"
	}
	if strings.HasSuffix(name, ".webp") {
		return "image/webp"
	}
	return "image/jpeg"
}
