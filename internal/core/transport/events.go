package transport

import (
	"context"
	"gogogot/internal/channel"
	"gogogot/internal/core/agent/event"

	"github.com/rs/zerolog/log"
)

var toolLabel = map[string]string{
	"bash":            "Running command",
	"edit_file":       "Editing file",
	"read_file":       "Reading file",
	"write_file":      "Writing file",
	"list_files":      "Listing files",
	"web_search":      "Searching the web",
	"web_fetch":       "Reading webpage",
	"web_request":     "Making request",
	"web_download":    "Downloading",
	"send_file":       "Sending file",
	"task_plan":       "Planning",
	"memory_read":     "Checking memory",
	"memory_write":    "Saving to memory",
	"memory_list":     "Listing memories",
	"recall":          "Recalling history",
	"schedule_add":    "Scheduling task",
	"schedule_list":   "Listing schedule",
	"schedule_remove": "Removing schedule",
	"soul_read":       "Reading identity",
	"soul_write":      "Updating identity",
	"user_read":       "Reading user profile",
	"user_write":      "Updating user profile",
	"system_info":     "Checking system",
	"skill_read":      "Reading skill",
	"skill_list":      "Listing skills",
	"skill_create":    "Creating skill",
	"skill_update":    "Updating skill",
	"skill_delete":    "Deleting skill",
}

func BuildToolStatus(d event.ToolStartData) channel.AgentStatus {
	label := toolLabel[d.Name]
	if label == "" {
		label = d.Name
	}
	if d.Detail != "" {
		label = label + ": " + d.Detail
	}

	phase := channel.PhaseTool
	if d.Name == "task_plan" {
		phase = channel.PhasePlanning
	}

	return channel.AgentStatus{Phase: phase, Tool: d.Name, Detail: label}
}

// ConsumeEvents reads agent events and translates them into channel
// interactions (typing, status updates, text). Returns the final text output.
func ConsumeEvents(ctx context.Context, reply channel.Replier, events <-chan event.Event, statusID string) string {
	var finalText string
	var toolsUsed []string

	for ev := range events {
		switch ev.Kind {
		case event.LLMStart:
			if statusID != "" {
				_ = reply.UpdateStatus(ctx, statusID, channel.AgentStatus{Phase: channel.PhaseThinking})
			}
			_ = reply.SendTyping(ctx)

		case event.LLMStream:
			if d, ok := ev.Data.(event.LLMStreamData); ok {
				finalText = d.Text
			}

		case event.ToolStart:
			d, _ := ev.Data.(event.ToolStartData)
			toolsUsed = append(toolsUsed, d.Name)
			log.Debug().Str("name", d.Name).Msg("transport: tool running")

			if statusID != "" {
				_ = reply.UpdateStatus(ctx, statusID, BuildToolStatus(d))
			}
			_ = reply.SendTyping(ctx)

		case event.Error:
			if ctx.Err() != nil {
				return ""
			}
			d, _ := ev.Data.(event.ErrorData)
			if statusID != "" {
				_ = reply.DeleteStatus(ctx, statusID)
			}
			_ = reply.SendText(ctx, "Error: "+d.Error)
			return ""

		case event.Done:
			cancelled := ctx.Err() != nil
			log.Info().
				Strs("tools_used", toolsUsed).
				Int("response_len", len(finalText)).
				Bool("cancelled", cancelled).
				Msg("transport: agent done")
			if statusID != "" {
				_ = reply.DeleteStatus(context.Background(), statusID)
			}
			if cancelled {
				return ""
			}
			return finalText
		}
	}
	return finalText
}
