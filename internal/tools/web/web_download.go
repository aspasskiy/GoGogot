package web

import (
	"context"
	"fmt"
	"github.com/aspasskiy/gogogot/internal/infra/utils"
	"github.com/aspasskiy/gogogot/internal/tools/types"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	downloadTimeout = 120 * time.Second
	maxDownloadSize = 100 * 1024 * 1024 // 100 MB
)

func WebDownloadTool() types.Tool {
	return types.Tool{
		Name:  "web_download",
		Label: "Downloading",
		Description: "Download a file from a URL and save it to disk. Use for fetching PDFs, images, archives, binaries, or any file. Max size 100 MB. If path is omitted, saves to /tmp with filename from URL.",
		Parameters: map[string]any{
			"url": map[string]any{
				"type":        "string",
				"description": "The full URL of the file to download",
			},
			"path": map[string]any{
				"type":        "string",
				"description": "Absolute path where to save the file. If omitted, auto-generates path in /tmp.",
			},
		},
		Required: []string{"url"},
		Handler:  webDownload,
	}
}

func webDownload(ctx context.Context, input map[string]any) types.Result {
	rawURL, err := types.GetString(input, "url")
	if err != nil {
		return types.ErrResult(err)
	}

	dest := types.GetStringOpt(input, "path")
	if dest == "" {
		parsed, err := url.Parse(rawURL)
		if err == nil {
			dest = filepath.Base(parsed.Path)
		}
		if dest == "" || dest == "." || dest == "/" {
			dest = "download"
		}
		dest = filepath.Join("/tmp", dest)
	}

	ctx, cancel := context.WithTimeout(ctx, downloadTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return types.Errf("bad url: %v", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; SofieBot/1.0)")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return types.Errf("http error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return types.Errf("HTTP %d for %s", resp.StatusCode, rawURL)
	}

	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return types.Errf("mkdir error: %v", err)
	}

	f, err := os.Create(dest)
	if err != nil {
		return types.Errf("create file error: %v", err)
	}
	defer f.Close()

	written, err := io.Copy(f, io.LimitReader(resp.Body, maxDownloadSize))
	if err != nil {
		os.Remove(dest)
		return types.Errf("download error: %v", err)
	}

	ct := resp.Header.Get("Content-Type")

	var sb strings.Builder
	fmt.Fprintf(&sb, "Downloaded to %s\n", dest)
	fmt.Fprintf(&sb, "Size: %s\n", utils.HumanSize(written))
	if ct != "" {
		fmt.Fprintf(&sb, "Content-Type: %s\n", ct)
	}
	return types.Result{Output: sb.String()}
}
