package telegram

import (
	"context"
	"gogogot/internal/core/transport"
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
	"report_status":   "Updating status",
	"send_message":    "Sending message",
	"ask_user":        "Asking user",
}

func buildToolStatus(d transport.ToolStartData, plan []transport.PlanTask) transport.AgentStatus {
	label := toolLabel[d.Name]
	if label == "" {
		label = d.Name
	}
	if d.Detail != "" {
		label = label + ": " + d.Detail
	}

	phase := transport.PhaseTool
	if d.Name == "task_plan" {
		phase = transport.PhasePlanning
	}

	return transport.AgentStatus{Phase: phase, Tool: d.Name, Detail: label, Plan: plan}
}

func formatMessageWithLevel(text string, level transport.MessageLevel) string {
	switch level {
	case transport.LevelSuccess:
		return "✅ " + text
	case transport.LevelWarning:
		return "⚠️ " + text
	default:
		return "💡 " + text
	}
}

func (r *replier) ConsumeEvents(ctx context.Context, events <-chan transport.Event, replyInbox <-chan string) string {
	_ = r.SendTyping(ctx)
	statusID, _ := r.sendStatus(ctx, transport.AgentStatus{Phase: transport.PhaseThinking})

	var (
		finalText   string
		currentPlan []transport.PlanTask
	)

	updateStatus := func(s transport.AgentStatus) {
		if statusID != "" {
			_ = r.updateStatus(ctx, statusID, s)
		}
	}

	restoreStatus := func() string {
		if statusID == "" {
			return ""
		}
		sid, _ := r.sendStatus(ctx, transport.AgentStatus{
			Phase: transport.PhaseWorking,
			Plan:  currentPlan,
		})
		return sid
	}

	for ev := range events {
		switch ev.Kind {
		case transport.LLMStart:
			updateStatus(transport.AgentStatus{Phase: transport.PhaseThinking, Plan: currentPlan})
			_ = r.SendTyping(ctx)

		case transport.LLMStream:
			if d, ok := ev.Data.(transport.LLMStreamData); ok {
				finalText = d.Text
			}

		case transport.ToolStart:
			d, _ := ev.Data.(transport.ToolStartData)
			updateStatus(buildToolStatus(d, currentPlan))
			_ = r.SendTyping(ctx)

		case transport.Progress:
			d, _ := ev.Data.(transport.ProgressData)
			if d.Tasks != nil {
				currentPlan = d.Tasks
			}
			status := transport.AgentStatus{
				Phase:   transport.PhaseWorking,
				Plan:    currentPlan,
				Detail:  d.Status,
				Percent: d.Percent,
			}
			updateStatus(status)

		case transport.Message:
			d, _ := ev.Data.(transport.MessageData)
			text := formatMessageWithLevel(d.Text, d.Level)
			updateStatus(transport.AgentStatus{Phase: transport.PhaseMessage, Detail: text, Plan: currentPlan})

		case transport.Ask:
			d, _ := ev.Data.(transport.AskData)
			if statusID != "" {
				_ = r.deleteStatus(ctx, statusID)
			}
			_ = r.SendAsk(ctx, d.Prompt, d.Kind, d.Options)

			if replyInbox != nil {
				select {
				case resp := <-replyInbox:
					if d.ReplyCh != nil {
						d.ReplyCh <- resp
					}
				case <-ctx.Done():
					if d.ReplyCh != nil {
						close(d.ReplyCh)
					}
					return ""
				}
			} else {
				if d.ReplyCh != nil {
					d.ReplyCh <- "(no interactive input available)"
				}
			}
			statusID = restoreStatus()

		case transport.Error:
			if ctx.Err() != nil {
				return ""
			}
			d, _ := ev.Data.(transport.ErrorData)
			if statusID != "" {
				_ = r.deleteStatus(ctx, statusID)
			}
			_ = r.SendText(ctx, "Error: "+d.Error)
			return ""

		case transport.Done:
			cancelled := ctx.Err() != nil
			if statusID != "" {
				_ = r.deleteStatus(context.Background(), statusID)
			}
			if cancelled {
				return ""
			}
			return finalText
		}
	}
	return finalText
}
