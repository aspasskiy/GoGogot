package system

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gogogot/tools"

	"github.com/rs/zerolog/log"
)

const maxFileSize = 50 * 1024

func FileTools() []tools.Tool {
	return []tools.Tool{
		{
			Name:        "read_file",
			Description: "Read the contents of a file at the given path.",
			Parameters: map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Absolute or relative path to the file",
				},
			},
			Required: []string{"path"},
			Handler:  readFile,
		},
		{
			Name:        "write_file",
			Description: "Write content to a file, creating parent directories as needed.",
			Parameters: map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Absolute or relative path to the file",
				},
				"content": map[string]any{
					"type":        "string",
					"description": "The content to write",
				},
			},
			Required: []string{"path", "content"},
			Handler:  writeFile,
		},
		{
			Name:        "list_files",
			Description: "List files and directories at the given path.",
			Parameters: map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Directory path to list (defaults to current directory)",
				},
			},
			Handler: listFiles,
		},
	}
}

func readFile(_ context.Context, input map[string]any) tools.Result {
	path, _ := input["path"].(string)
	if path == "" {
		return tools.Result{Output: "path is required", IsErr: true}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		log.Debug().Str("path", path).Err(err).Msg("read_file error")
		return tools.Result{Output: fmt.Sprintf("read error: %v", err), IsErr: true}
	}

	truncated := len(data) > maxFileSize
	if truncated {
		data = data[:maxFileSize]
	}
	log.Debug().Str("path", path).Int("size", len(data)).Bool("truncated", truncated).Msg("read_file")

	if truncated {
		return tools.Result{Output: string(data) + "\n... (file truncated)"}
	}
	return tools.Result{Output: string(data)}
}

func writeFile(_ context.Context, input map[string]any) tools.Result {
	path, _ := input["path"].(string)
	content, _ := input["content"].(string)
	if path == "" {
		return tools.Result{Output: "path is required", IsErr: true}
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Debug().Str("dir", dir).Err(err).Msg("write_file mkdir error")
		return tools.Result{Output: fmt.Sprintf("mkdir error: %v", err), IsErr: true}
	}

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		log.Debug().Str("path", path).Err(err).Msg("write_file error")
		return tools.Result{Output: fmt.Sprintf("write error: %v", err), IsErr: true}
	}
	log.Debug().Str("path", path).Int("bytes", len(content)).Msg("write_file")
	return tools.Result{Output: fmt.Sprintf("wrote %d bytes to %s", len(content), path)}
}

func listFiles(_ context.Context, input map[string]any) tools.Result {
	path, _ := input["path"].(string)
	if path == "" {
		path = "."
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		log.Debug().Str("path", path).Err(err).Msg("list_files error")
		return tools.Result{Output: fmt.Sprintf("readdir error: %v", err), IsErr: true}
	}

	log.Debug().Str("path", path).Int("entries", len(entries)).Msg("list_files")

	var b strings.Builder
	for _, e := range entries {
		suffix := ""
		if e.IsDir() {
			suffix = "/"
		}
		fmt.Fprintf(&b, "%s%s\n", e.Name(), suffix)
	}
	if b.Len() == 0 {
		return tools.Result{Output: "(empty directory)"}
	}
	return tools.Result{Output: b.String()}
}
