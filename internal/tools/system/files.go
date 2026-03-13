package system

import (
	"context"
	"fmt"
	"gogogot/internal/tools/types"
	"os"
	"path/filepath"
	"strings"
)

func pathDetail(input map[string]any) string {
	if p, ok := input["path"].(string); ok {
		return filepath.Base(p)
	}
	return ""
}

func FileTools() []types.Tool {
	return []types.Tool{
		{
			Name:        "read_file",
			Label:       "Reading file",
			Description: "Read the contents of a file at the given path.",
			DetailFunc:  pathDetail,
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
			Label:       "Writing file",
			Description: "Write content to a file, creating parent directories as needed.",
			DetailFunc:  pathDetail,
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
			Label:       "Listing files",
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

func readFile(_ context.Context, input map[string]any) types.Result {
	path, err := types.GetString(input, "path")
	if err != nil {
		return types.ErrResult(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return types.Errf("read error: %v", err)
	}

	return types.TruncateOutput(string(data))
}

func writeFile(_ context.Context, input map[string]any) types.Result {
	path, err := types.GetString(input, "path")
	if err != nil {
		return types.ErrResult(err)
	}
	content := types.GetStringOpt(input, "content")

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return types.Errf("mkdir error: %v", err)
	}

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return types.Errf("write error: %v", err)
	}
	return types.Result{Output: fmt.Sprintf("wrote %d bytes to %s", len(content), path)}
}

func listFiles(_ context.Context, input map[string]any) types.Result {
	path := types.GetStringOpt(input, "path")
	if path == "" {
		path = "."
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return types.Errf("readdir error: %v", err)
	}

	var b strings.Builder
	for _, e := range entries {
		suffix := ""
		if e.IsDir() {
			suffix = "/"
		}
		fmt.Fprintf(&b, "%s%s\n", e.Name(), suffix)
	}
	if b.Len() == 0 {
		return types.Result{Output: "(empty directory)"}
	}
	return types.Result{Output: b.String()}
}
