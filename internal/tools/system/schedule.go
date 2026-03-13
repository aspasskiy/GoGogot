package system

import (
	"context"
	"encoding/json"
	"fmt"
	"gogogot/internal/infra/scheduler"
	"gogogot/internal/tools/types"
)

func ScheduleTools(sched *scheduler.Scheduler) []types.Tool {
	return []types.Tool{
		{
			Name:  "schedule_add",
			Label: "Scheduling task",
			Description: "Add or update a recurring scheduled task. When the task fires, YOU wake up with full access to all your tools, memory, and skills to execute the command. Persists across restarts. Use standard 5-field cron: min hour dom month dow.",
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
					"description": "Natural-language instruction for yourself (NOT a shell command). Describe what you should do when woken up. Example: 'Check server health and send a summary to the owner'",
				},
				"skill": map[string]any{
					"type":        "string",
					"description": "Optional skill name to follow when this task fires. The skill will be read with skill_read automatically. Use this for complex multi-step procedures.",
				},
				"label": map[string]any{
					"type":        "string",
					"description": "Short human-readable label for display",
				},
			},
			Required: []string{"id", "schedule", "command"},
			Handler: func(_ context.Context, input map[string]any) types.Result {
				if sched == nil {
					return types.Result{Output: "scheduler not available", IsErr: true}
				}
				id, err := types.GetString(input, "id")
				if err != nil {
					return types.ErrResult(err)
				}
				schedule, err := types.GetString(input, "schedule")
				if err != nil {
					return types.ErrResult(err)
				}
				command, err := types.GetString(input, "command")
				if err != nil {
					return types.ErrResult(err)
				}
				skill := types.GetStringOpt(input, "skill")
				label := types.GetStringOpt(input, "label")
				if err := sched.Add(id, schedule, command, skill, label); err != nil {
					return types.Errf("failed to add schedule: %v", err)
				}
				out := fmt.Sprintf("scheduled task %q with cron %q: %s", id, schedule, command)
				if skill != "" {
					out += fmt.Sprintf(" (skill: %s)", skill)
				}
				return types.Result{Output: out}
			},
		},
		{
			Name:  "schedule_list",
			Label: "Listing schedule",
			Description: "List all scheduled recurring tasks with their cron expressions, commands, next run times, and execution state (last status, errors, duration).",
			Parameters:  map[string]any{},
			Handler: func(_ context.Context, _ map[string]any) types.Result {
				if sched == nil {
					return types.Result{Output: "scheduler not available", IsErr: true}
				}
				tasks := sched.List()
				if len(tasks) == 0 {
					return types.Result{Output: "(no scheduled tasks)"}
				}
				data, err := json.MarshalIndent(tasks, "", "  ")
				if err != nil {
					return types.Errf("marshal error: %v", err)
				}
				return types.Result{Output: string(data)}
			},
		},
		{
			Name:  "schedule_remove",
			Label: "Removing schedule",
			Description: "Remove a scheduled recurring task by its ID.",
			Parameters: map[string]any{
				"id": map[string]any{
					"type":        "string",
					"description": "The task ID to remove",
				},
			},
			Required: []string{"id"},
			Handler: func(_ context.Context, input map[string]any) types.Result {
				if sched == nil {
					return types.Result{Output: "scheduler not available", IsErr: true}
				}
				id, err := types.GetString(input, "id")
				if err != nil {
					return types.ErrResult(err)
				}
				if err := sched.Remove(id); err != nil {
					return types.Errf("failed to remove: %v", err)
				}
				return types.Result{Output: fmt.Sprintf("removed scheduled task %q", id)}
			},
		},
	}
}
