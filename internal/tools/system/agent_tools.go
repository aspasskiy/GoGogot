package system

import (
	"context"
	"github.com/aspasskiy/gogogot/internal/core/transport"
	"github.com/aspasskiy/gogogot/internal/tools/types"
)

func AgentTools(tp *TaskPlan) []types.Tool {
	return []types.Tool{
		TaskPlanTool(tp),
		reportStatusTool(),
		sendMessageTool(),
		askUserTool(),
	}
}

func reportStatusTool() types.Tool {
	return types.Tool{
		Name:        "report_status",
		Label:       "Updating status",
		Description: "Update the visible status shown to the user. Use during long tasks to communicate what you are working on.",
		Parameters: map[string]any{
			"text": map[string]any{
				"type":        "string",
				"description": "Short status text, e.g. 'Analyzing Russian market data...'",
			},
			"percent": map[string]any{
				"type":        "integer",
				"description": "Optional progress percentage 0-100 for measurable operations",
			},
		},
		Required: []string{"text"},
		Handler: func(ctx context.Context, input map[string]any) types.Result {
			text, _ := input["text"].(string)
			var pct *int
			if v, ok := input["percent"].(float64); ok {
				i := int(v)
				pct = &i
			}
			if bus, ok := transport.BusFromContext(ctx); ok {
				bus.Emit(transport.Progress, transport.ProgressData{Status: text, Percent: pct})
			}
			return types.Result{Output: "status updated"}
		},
	}
}

func sendMessageTool() types.Tool {
	return types.Tool{
		Name:        "send_message",
		Label:       "Sending message",
		Description: "Send an intermediate message to the user without ending your current task. Use to share findings, progress updates, or important information mid-run.",
		Parameters: map[string]any{
			"text": map[string]any{
				"type":        "string",
				"description": "Message text to send",
			},
			"level": map[string]any{
				"type":        "string",
				"enum":        []string{"info", "success", "warning"},
				"description": "Optional message level: info (default), success, or warning",
			},
		},
		Required: []string{"text"},
		Handler: func(ctx context.Context, input map[string]any) types.Result {
			text, _ := input["text"].(string)
			level, _ := input["level"].(string)
			if bus, ok := transport.BusFromContext(ctx); ok {
				bus.Emit(transport.Message, transport.MessageData{
					Text:  text,
					Level: transport.MessageLevel(level),
				})
			}
			return types.Result{Output: "message sent"}
		},
	}
}

func askUserTool() types.Tool {
	return types.Tool{
		Name:        "ask_user",
		Label:       "Asking user",
		Interactive: true,
		Description: "Ask the user a question and wait for their response. Use for clarification, confirmation, or choices. The agent pauses until the user replies.",
		Parameters: map[string]any{
			"question": map[string]any{
				"type":        "string",
				"description": "The question to ask",
			},
			"kind": map[string]any{
				"type":        "string",
				"enum":        []string{"freeform", "confirm", "choice"},
				"description": "Interaction type: freeform (open text, default), confirm (yes/no), choice (pick from options)",
			},
			"options": map[string]any{
				"type":        "array",
				"description": "For 'choice' kind: array of {value, label} objects",
			},
		},
		Required: []string{"question"},
		Handler: func(_ context.Context, _ map[string]any) types.Result {
			return types.Result{Output: "ok"}
		},
	}
}
