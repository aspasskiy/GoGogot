package tools

import (
	"context"
	"fmt"
	"github.com/aspasskiy/gogogot/internal/tools/store"
	"github.com/aspasskiy/gogogot/internal/tools/types"
	"strings"
)

func MemoryTools(st store.Store) []types.Tool {
	return []types.Tool{
		{
			Name:  "memory_list",
			Label: "Listing memories",
			Description: "List all files in your persistent memory. Memory survives across all conversations. Check this at the start of each conversation to recall what you know.",
			Parameters:  map[string]any{},
			Handler: func(_ context.Context, _ map[string]any) types.Result {
				files, err := st.ListMemory()
				if err != nil {
					return types.Result{Output: "error listing memory: " + err.Error(), IsErr: true}
				}
				if len(files) == 0 {
					return types.Result{Output: "(no memory files yet)"}
				}
				var b strings.Builder
				for _, f := range files {
					fmt.Fprintf(&b, "%s  (%d bytes)\n", f.Name, f.Size)
				}
				return types.Result{Output: b.String()}
			},
		},
		{
			Name:  "memory_read",
			Label: "Checking memory",
			Description: "Read a specific memory file. Your memory is organized as markdown files by topic.",
			Parameters: map[string]any{
				"file": map[string]any{
					"type":        "string",
					"description": "Name of the memory file to read, e.g. owner.md",
				},
			},
			Required: []string{"file"},
			Handler: func(_ context.Context, input map[string]any) types.Result {
				file, err := types.GetString(input, "file")
				if err != nil {
					return types.ErrResult(err)
				}
				content, err := st.ReadMemory(file)
				if err != nil {
					return types.Result{Output: err.Error(), IsErr: true}
				}
				return types.Result{Output: content}
			},
		},
		{
			Name:  "memory_write",
			Label: "Saving to memory",
			Description: "Write or update a memory file. Organize knowledge into topic files (e.g. owner.md, server.md, tasks.md). You decide the structure. Update files incrementally — read first, then write the improved version.",
			Parameters: map[string]any{
				"file": map[string]any{
					"type":        "string",
					"description": "Name of the memory file, e.g. owner.md",
				},
				"content": map[string]any{
					"type":        "string",
					"description": "The full content for this memory file in markdown format",
				},
			},
			Required: []string{"file", "content"},
			Handler: func(_ context.Context, input map[string]any) types.Result {
				file, err := types.GetString(input, "file")
				if err != nil {
					return types.ErrResult(err)
				}
				content, err := types.GetString(input, "content")
				if err != nil {
					return types.ErrResult(err)
				}
				if err := st.WriteMemory(file, content); err != nil {
					return types.Result{Output: "error writing memory: " + err.Error(), IsErr: true}
				}
				return types.Result{Output: fmt.Sprintf("memory file %q updated (%d bytes)", file, len(content))}
			},
		},
		{
			Name:  "memory_delete",
			Label: "Deleting memory",
			Description: "Delete a memory file that is no longer needed. Use to prune outdated or irrelevant information.",
			Parameters: map[string]any{
				"file": map[string]any{
					"type":        "string",
					"description": "Name of the memory file to delete, e.g. old_tasks.md",
				},
			},
			Required: []string{"file"},
			Handler: func(_ context.Context, input map[string]any) types.Result {
				file, err := types.GetString(input, "file")
				if err != nil {
					return types.ErrResult(err)
				}
				if err := st.DeleteMemory(file); err != nil {
					return types.Result{Output: "error deleting memory: " + err.Error(), IsErr: true}
				}
				return types.Result{Output: fmt.Sprintf("memory file %q deleted", file)}
			},
		},
	}
}
