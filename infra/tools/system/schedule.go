package system

import (
	"context"
	"encoding/json"
	"fmt"
	"gogogot/infra/tools"

	"gogogot/infra/scheduler"
)

func ScheduleTools(sched *scheduler.Scheduler) []tools.Tool {
	return []tools.Tool{
		{
			Name:        "schedule_add",
			Description: "Add or update a recurring scheduled task. The task runs in the owner's active chat on the cron schedule, preserving full conversation context. Persists across restarts. Use standard 5-field cron: min hour dom month dow.",
			Parameters: map[string]any{
				"id": map[string]any{
					"type":        "string",
					"description": "Unique identifier for this task, e.g. 'morning-news', 'daily-backup'",
				},
				"schedule": map[string]any{
					"type":        "string",
					"description": "Cron expression (5-field): minute hour day-of-month month day-of-week. Examples: '0 8 * * *' (daily 8am), '*/30 * * * *' (every 30min), '0 9 * * 1' (Mon 9am)",
				},
				"command": map[string]any{
					"type":        "string",
					"description": "Task description — what the agent should do when triggered",
				},
				"label": map[string]any{
					"type":        "string",
					"description": "Short human-readable label for display",
				},
			},
			Required: []string{"id", "schedule", "command"},
		Handler: func(_ context.Context, input map[string]any) tools.Result {
			if sched == nil {
				return tools.Result{Output: "scheduler not available", IsErr: true}
			}
			id, err := tools.GetString(input, "id")
			if err != nil {
				return tools.ErrResult(err)
			}
			schedule, err := tools.GetString(input, "schedule")
			if err != nil {
				return tools.ErrResult(err)
			}
			command, err := tools.GetString(input, "command")
			if err != nil {
				return tools.ErrResult(err)
			}
			label := tools.GetStringOpt(input, "label")
			if err := sched.Add(id, schedule, command, label); err != nil {
				return tools.Result{Output: fmt.Sprintf("failed to add schedule: %v", err), IsErr: true}
			}
			return tools.Result{Output: fmt.Sprintf("scheduled task %q with cron %q: %s", id, schedule, command)}
			},
		},
		{
			Name:        "schedule_list",
			Description: "List all scheduled recurring tasks with their cron expressions, commands, next run times, and execution state (last status, errors, duration).",
			Parameters:  map[string]any{},
		Handler: func(_ context.Context, _ map[string]any) tools.Result {
			if sched == nil {
				return tools.Result{Output: "scheduler not available", IsErr: true}
			}
			tasks := sched.List()
			if len(tasks) == 0 {
				return tools.Result{Output: "(no scheduled tasks)"}
			}
			data, err := json.MarshalIndent(tasks, "", "  ")
			if err != nil {
				return tools.Result{Output: fmt.Sprintf("marshal error: %v", err), IsErr: true}
			}
			return tools.Result{Output: string(data)}
			},
		},
		{
			Name:        "schedule_remove",
			Description: "Remove a scheduled recurring task by its ID.",
			Parameters: map[string]any{
				"id": map[string]any{
					"type":        "string",
					"description": "The task ID to remove",
				},
			},
			Required: []string{"id"},
		Handler: func(_ context.Context, input map[string]any) tools.Result {
			if sched == nil {
				return tools.Result{Output: "scheduler not available", IsErr: true}
			}
			id, err := tools.GetString(input, "id")
			if err != nil {
				return tools.ErrResult(err)
			}
			if err := sched.Remove(id); err != nil {
				return tools.Result{Output: fmt.Sprintf("failed to remove: %v", err), IsErr: true}
			}
			return tools.Result{Output: fmt.Sprintf("removed scheduled task %q", id)}
			},
		},
	}
}
