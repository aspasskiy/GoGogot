package system

import (
	"context"
	"fmt"
	"strings"

	"gogogot/core/store"
	"gogogot/infra/tools"

	"github.com/rs/zerolog/log"
)

func MemoryTools() []tools.Tool {
	return []tools.Tool{
		{
			Name:        "memory_list",
			Description: "List all files in your persistent memory. Memory survives across all conversations. Check this at the start of each conversation to recall what you know.",
			Parameters:  map[string]any{},
			Handler:     memoryList,
		},
		{
			Name:        "memory_read",
			Description: "Read a specific memory file. Your memory is organized as markdown files by topic.",
			Parameters: map[string]any{
				"file": map[string]any{
					"type":        "string",
					"description": "Name of the memory file to read, e.g. owner.md",
				},
			},
			Required: []string{"file"},
			Handler:  memoryRead,
		},
		{
			Name:        "memory_write",
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
			Handler:  memoryWrite,
		},
	}
}

func memoryList(_ context.Context, _ map[string]any) tools.Result {
	files, err := store.ListMemory()
	if err != nil {
		log.Debug().Err(err).Msg("memory_list error")
		return tools.Result{Output: "error listing memory: " + err.Error(), IsErr: true}
	}
	if len(files) == 0 {
		return tools.Result{Output: "(no memory files yet)"}
	}
	var b strings.Builder
	for _, f := range files {
		fmt.Fprintf(&b, "%s  (%d bytes)\n", f.Name, f.Size)
	}
	return tools.Result{Output: b.String()}
}

func memoryRead(_ context.Context, input map[string]any) tools.Result {
	file, err := tools.GetString(input, "file")
	if err != nil {
		return tools.ErrResult(err)
	}
	content, err := store.ReadMemory(file)
	if err != nil {
		log.Debug().Str("file", file).Err(err).Msg("memory_read error")
		return tools.Result{Output: err.Error(), IsErr: true}
	}
	return tools.Result{Output: content}
}

func memoryWrite(_ context.Context, input map[string]any) tools.Result {
	file, err := tools.GetString(input, "file")
	if err != nil {
		return tools.ErrResult(err)
	}
	content, err := tools.GetString(input, "content")
	if err != nil {
		return tools.ErrResult(err)
	}
	if err := store.WriteMemory(file, content); err != nil {
		log.Error().Err(err).Str("file", file).Msg("memory_write failed")
		return tools.Result{Output: "error writing memory: " + err.Error(), IsErr: true}
	}
	log.Info().Str("file", file).Int("content_len", len(content)).Msg("memory_write")
	return tools.Result{Output: fmt.Sprintf("memory file %q updated (%d bytes)", file, len(content))}
}
