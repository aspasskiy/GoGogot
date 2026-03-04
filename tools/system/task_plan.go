package system

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"gogogot/tools"
)

type taskStatus string

const (
	taskPending    taskStatus = "pending"
	taskInProgress taskStatus = "in_progress"
	taskCompleted  taskStatus = "completed"
)

func parseTaskStatus(s string) (taskStatus, bool) {
	switch taskStatus(s) {
	case taskPending, taskInProgress, taskCompleted:
		return taskStatus(s), true
	default:
		return "", false
	}
}

type taskItem struct {
	ID     int
	Title  string
	Status taskStatus
}

// TaskPlan holds session-scoped task state. Create one per agent session.
type TaskPlan struct {
	mu     sync.Mutex
	tasks  []taskItem
	nextID int
}

func NewTaskPlan() *TaskPlan {
	return &TaskPlan{nextID: 1}
}

func (tp *TaskPlan) create(entries []map[string]any) tools.Result {
	tp.mu.Lock()
	defer tp.mu.Unlock()

	items := make([]taskItem, 0, len(entries))
	id := 1
	for _, e := range entries {
		title, _ := e["title"].(string)
		if title == "" {
			return tools.Result{Output: "each task must have a non-empty 'title'", IsErr: true}
		}
		status := taskPending
		if s, ok := e["status"].(string); ok && s != "" {
			parsed, valid := parseTaskStatus(s)
			if !valid {
				return tools.Result{Output: fmt.Sprintf("invalid status %q; use pending, in_progress, or completed", s), IsErr: true}
			}
			status = parsed
		}
		items = append(items, taskItem{ID: id, Title: title, Status: status})
		id++
	}

	tp.tasks = items
	tp.nextID = id
	return tools.Result{Output: fmt.Sprintf("Created %d task(s).", len(items))}
}

func (tp *TaskPlan) add(title string) tools.Result {
	if title == "" {
		return tools.Result{Output: "'title' is required", IsErr: true}
	}
	tp.mu.Lock()
	defer tp.mu.Unlock()

	id := tp.nextID
	tp.nextID++
	tp.tasks = append(tp.tasks, taskItem{ID: id, Title: title, Status: taskPending})
	return tools.Result{Output: fmt.Sprintf("Added task [%d] %q.", id, title)}
}

func (tp *TaskPlan) update(id int, statusStr string) tools.Result {
	status, ok := parseTaskStatus(statusStr)
	if !ok {
		return tools.Result{Output: fmt.Sprintf("invalid status %q; use pending, in_progress, or completed", statusStr), IsErr: true}
	}
	tp.mu.Lock()
	defer tp.mu.Unlock()

	for i := range tp.tasks {
		if tp.tasks[i].ID == id {
			tp.tasks[i].Status = status
			return tools.Result{Output: fmt.Sprintf("Task [%d] updated to %s.", id, status)}
		}
	}
	return tools.Result{Output: fmt.Sprintf("task with id %d not found", id), IsErr: true}
}

func (tp *TaskPlan) list() tools.Result {
	tp.mu.Lock()
	defer tp.mu.Unlock()

	if len(tp.tasks) == 0 {
		return tools.Result{Output: "No tasks."}
	}

	completed := 0
	for _, t := range tp.tasks {
		if t.Status == taskCompleted {
			completed++
		}
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Tasks (%d/%d completed):", completed, len(tp.tasks))
	for _, t := range tp.tasks {
		fmt.Fprintf(&sb, "\n- [%d] [%s] %s", t.ID, t.Status, t.Title)
	}
	return tools.Result{Output: sb.String()}
}

func (tp *TaskPlan) deleteAll() tools.Result {
	tp.mu.Lock()
	defer tp.mu.Unlock()

	tp.tasks = nil
	tp.nextID = 1
	return tools.Result{Output: "Task list cleared."}
}

// TaskPlanTool returns the task_plan tool wired to the given TaskPlan state.
func TaskPlanTool(tp *TaskPlan) tools.Tool {
	return tools.Tool{
		Name: "task_plan",
		Description: "Manage a session task checklist. Use to break complex work into steps and track progress. " +
			"Actions: create (batch-replace list), add (append one task), update (change status), list (show all), delete (clear all).",
		Parameters: map[string]any{
			"action": map[string]any{
				"type":        "string",
				"enum":        []string{"create", "add", "update", "list", "delete"},
				"description": "Operation to perform",
			},
			"tasks": map[string]any{
				"type":        "array",
				"description": "For 'create': array of {title, status?} objects (replaces existing list)",
			},
			"title": map[string]any{
				"type":        "string",
				"description": "For 'add': title of the new task",
			},
			"id": map[string]any{
				"type":        "integer",
				"description": "For 'update': ID of the task to update",
			},
			"status": map[string]any{
				"type":        "string",
				"enum":        []string{"pending", "in_progress", "completed"},
				"description": "For 'update': new status",
			},
		},
		Required: []string{"action"},
		Handler: func(_ context.Context, input map[string]any) tools.Result {
			action, _ := input["action"].(string)
			switch action {
			case "create":
				raw, ok := input["tasks"].([]any)
				if !ok || len(raw) == 0 {
					return tools.Result{Output: "'tasks' must be a non-empty array of {title, status?}", IsErr: true}
				}
				entries := make([]map[string]any, 0, len(raw))
				for _, r := range raw {
					if m, ok := r.(map[string]any); ok {
						entries = append(entries, m)
					}
				}
				if len(entries) == 0 {
					return tools.Result{Output: "'tasks' entries must be objects with a 'title' field", IsErr: true}
				}
				return tp.create(entries)

			case "add":
				title, _ := input["title"].(string)
				return tp.add(title)

			case "update":
				idF, _ := input["id"].(float64)
				id := int(idF)
				status, _ := input["status"].(string)
				if id == 0 {
					return tools.Result{Output: "'id' is required for update", IsErr: true}
				}
				if status == "" {
					return tools.Result{Output: "'status' is required for update", IsErr: true}
				}
				return tp.update(id, status)

			case "list":
				return tp.list()

			case "delete":
				return tp.deleteAll()

			default:
				return tools.Result{
					Output: fmt.Sprintf("unknown action %q; use create, add, update, list, or delete", action),
					IsErr:  true,
				}
			}
		},
	}
}
