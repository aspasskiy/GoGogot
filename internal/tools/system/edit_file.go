package system

import (
	"context"
	"fmt"
	"gogogot/internal/tools/types"
	"os"
	"strings"
)

func EditFileTool() types.Tool {
	return types.Tool{
		Name:        "edit_file",
		Label:       "Editing file",
		Description: "Edit a file by replacing a specific string with a new one. Safer than write_file for modifying configs — only the matched portion changes. Returns error if old_string is not found.",
		DetailFunc: pathDetail,
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

func editFile(_ context.Context, input map[string]any) types.Result {
	path, err := types.GetString(input, "path")
	if err != nil {
		return types.ErrResult(err)
	}
	oldStr, err := types.GetString(input, "old_string")
	if err != nil {
		return types.ErrResult(err)
	}
	newStr, err := types.GetString(input, "new_string")
	if err != nil {
		return types.ErrResult(err)
	}
	replaceAll := types.GetBool(input, "replace_all")

	data, err := os.ReadFile(path)
	if err != nil {
		return types.Errf("read error: %v", err)
	}

	content := string(data)
	count := strings.Count(content, oldStr)
	if count == 0 {
		return types.Result{Output: "old_string not found in file", IsErr: true}
	}

	var updated string
	if replaceAll {
		updated = strings.ReplaceAll(content, oldStr, newStr)
	} else {
		updated = strings.Replace(content, oldStr, newStr, 1)
	}

	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		return types.Errf("write error: %v", err)
	}

	replaced := count
	if !replaceAll {
		replaced = 1
	}
	return types.Result{Output: fmt.Sprintf("replaced %d occurrence(s) in %s", replaced, path)}
}
