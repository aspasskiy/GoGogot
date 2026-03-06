package system

import (
	"context"
	"fmt"
	"os"
	"strings"

	"gogogot/infra/tools"

	"github.com/rs/zerolog/log"
)

func EditFileTool() tools.Tool {
	return tools.Tool{
		Name:        "edit_file",
		Description: "Edit a file by replacing a specific string with a new one. Safer than write_file for modifying configs — only the matched portion changes. Returns error if old_string is not found.",
		Parameters: map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Absolute or relative path to the file",
			},
			"old_string": map[string]any{
				"type":        "string",
				"description": "The exact string to find in the file",
			},
			"new_string": map[string]any{
				"type":        "string",
				"description": "The replacement string",
			},
			"replace_all": map[string]any{
				"type":        "boolean",
				"description": "If true, replace all occurrences. Default is false (first occurrence only).",
			},
		},
		Required: []string{"path", "old_string", "new_string"},
		Handler:  editFile,
	}
}

func editFile(_ context.Context, input map[string]any) tools.Result {
	path, err := tools.GetString(input, "path")
	if err != nil {
		return tools.ErrResult(err)
	}
	oldStr, err := tools.GetString(input, "old_string")
	if err != nil {
		return tools.ErrResult(err)
	}
	newStr := tools.GetStringOpt(input, "new_string")
	replaceAll := tools.GetBool(input, "replace_all")

	data, err := os.ReadFile(path)
	if err != nil {
		log.Debug().Str("path", path).Err(err).Msg("edit_file read error")
		return tools.Result{Output: fmt.Sprintf("read error: %v", err), IsErr: true}
	}

	content := string(data)
	count := strings.Count(content, oldStr)
	if count == 0 {
		return tools.Result{Output: "old_string not found in file", IsErr: true}
	}

	var updated string
	if replaceAll {
		updated = strings.ReplaceAll(content, oldStr, newStr)
	} else {
		updated = strings.Replace(content, oldStr, newStr, 1)
	}

	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		log.Debug().Str("path", path).Err(err).Msg("edit_file write error")
		return tools.Result{Output: fmt.Sprintf("write error: %v", err), IsErr: true}
	}

	replaced := count
	if !replaceAll {
		replaced = 1
	}
	log.Debug().Str("path", path).Int("occurrences", count).Int("replaced", replaced).Msg("edit_file")
	return tools.Result{Output: fmt.Sprintf("replaced %d occurrence(s) in %s", replaced, path)}
}
