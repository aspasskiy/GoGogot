package main

import (
	"context"
	"fmt"
	"github.com/aspasskiy/gogogot/internal/channel/telegram"
	"github.com/aspasskiy/gogogot/internal/core/transport"
	"os"
	"strconv"
	"time"
)

func main() {
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	ownerStr := os.Getenv("TELEGRAM_OWNER_ID")
	if token == "" || ownerStr == "" {
		fmt.Fprintln(os.Stderr, "set TELEGRAM_BOT_TOKEN and TELEGRAM_OWNER_ID")
		os.Exit(1)
	}
	ownerID, err := strconv.ParseInt(ownerStr, 10, 64)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid TELEGRAM_OWNER_ID: %v\n", err)
		os.Exit(1)
	}

	ch, err := telegram.New(token, ownerID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "telegram init: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()
	reply := ch.OwnerReplier()

	scenarioProgress(ctx, reply)

	fmt.Println("\nDone!")
}

func emit(ch chan<- transport.Event, kind transport.Kind, data any) {
	ch <- transport.Event{Timestamp: time.Now(), Kind: kind, Data: data}
}

func pct(v int) *int { return &v }

func scenarioProgress(ctx context.Context, reply transport.Replier) {
	fmt.Println(">>> Progress demo")
	events := make(chan transport.Event, 50)

	plan := func(tasks []transport.PlanTask, percent int, status string) transport.ProgressData {
		return transport.ProgressData{Tasks: tasks, Percent: pct(percent), Status: status}
	}

	go func() {
		defer close(events)
		step := 3 * time.Second

		// 1) Thinking
		emit(events, transport.LLMStart, nil)
		fmt.Println("    [thinking]")
		time.Sleep(step)

		// 2) Tool: analyzing codebase
		emit(events, transport.ToolStart, transport.ToolStartData{
			Name: "file_read", Label: "Reading files", Detail: "Analyzing codebase structure",
		})
		fmt.Println("    [tool: file_read]")
		time.Sleep(step)

		// 3) Plan appears, 0%
		tasks := []transport.PlanTask{
			{Title: "Analyze existing code", Status: transport.TaskInProgress},
			{Title: "Implement new handler", Status: transport.TaskPending},
			{Title: "Write unit tests", Status: transport.TaskPending},
			{Title: "Update configuration", Status: transport.TaskPending},
			{Title: "Run linter & build", Status: transport.TaskPending},
		}
		emit(events, transport.Progress, plan(tasks, 0, "Analyzing existing code"))
		fmt.Println("    [plan 0%]")
		time.Sleep(step)

		// 4) 10%
		emit(events, transport.Progress, plan(tasks, 10, "Reading source files"))
		fmt.Println("    [plan 10%]")
		time.Sleep(step)

		// 5) Task 1 done, task 2 starts, 20%
		tasks = []transport.PlanTask{
			{Title: "Analyze existing code", Status: transport.TaskCompleted},
			{Title: "Implement new handler", Status: transport.TaskInProgress},
			{Title: "Write unit tests", Status: transport.TaskPending},
			{Title: "Update configuration", Status: transport.TaskPending},
			{Title: "Run linter & build", Status: transport.TaskPending},
		}
		emit(events, transport.Progress, plan(tasks, 20, "Implementing handler"))
		fmt.Println("    [plan 20%]")
		time.Sleep(step)

		// 6) Tool: editing file
		emit(events, transport.ToolStart, transport.ToolStartData{
			Name: "file_edit", Label: "Editing file", Detail: "internal/api/handler.go",
		})
		fmt.Println("    [tool: file_edit]")
		time.Sleep(step)

		// 7) 40%
		emit(events, transport.Progress, plan(tasks, 40, "Handler implementation in progress"))
		fmt.Println("    [plan 40%]")
		time.Sleep(step)

		// 8) Message: success
		emit(events, transport.Message, transport.MessageData{
			Text: "Handler created successfully", Level: transport.LevelSuccess,
		})
		fmt.Println("    [message: success]")
		time.Sleep(step)

		// 9) Task 2 done, task 3 starts, 50%
		tasks = []transport.PlanTask{
			{Title: "Analyze existing code", Status: transport.TaskCompleted},
			{Title: "Implement new handler", Status: transport.TaskCompleted},
			{Title: "Write unit tests", Status: transport.TaskInProgress},
			{Title: "Update configuration", Status: transport.TaskPending},
			{Title: "Run linter & build", Status: transport.TaskPending},
		}
		emit(events, transport.Progress, plan(tasks, 50, "Writing tests"))
		fmt.Println("    [plan 50%]")
		time.Sleep(step)

		// 10) Tool: writing test file
		emit(events, transport.ToolStart, transport.ToolStartData{
			Name: "file_edit", Label: "Editing file", Detail: "internal/api/handler_test.go",
		})
		fmt.Println("    [tool: file_edit test]")
		time.Sleep(step)

		// 11) 65%
		emit(events, transport.Progress, plan(tasks, 65, "Tests written"))
		fmt.Println("    [plan 65%]")
		time.Sleep(step)

		// 12) Task 3 done, task 4 starts, 75%
		tasks = []transport.PlanTask{
			{Title: "Analyze existing code", Status: transport.TaskCompleted},
			{Title: "Implement new handler", Status: transport.TaskCompleted},
			{Title: "Write unit tests", Status: transport.TaskCompleted},
			{Title: "Update configuration", Status: transport.TaskInProgress},
			{Title: "Run linter & build", Status: transport.TaskPending},
		}
		emit(events, transport.Progress, plan(tasks, 75, "Updating config"))
		fmt.Println("    [plan 75%]")
		time.Sleep(step)

		// 13) Message: warning
		emit(events, transport.Message, transport.MessageData{
			Text: "Deprecated config key detected, migrating automatically", Level: transport.LevelWarning,
		})
		fmt.Println("    [message: warning]")
		time.Sleep(step)

		// 14) Task 4 done, task 5 starts, 85%
		tasks = []transport.PlanTask{
			{Title: "Analyze existing code", Status: transport.TaskCompleted},
			{Title: "Implement new handler", Status: transport.TaskCompleted},
			{Title: "Write unit tests", Status: transport.TaskCompleted},
			{Title: "Update configuration", Status: transport.TaskCompleted},
			{Title: "Run linter & build", Status: transport.TaskInProgress},
		}
		emit(events, transport.Progress, plan(tasks, 85, "Running go build"))
		fmt.Println("    [plan 85%]")
		time.Sleep(step)

		// 15) Tool: shell
		emit(events, transport.ToolStart, transport.ToolStartData{
			Name: "shell_exec", Label: "Running command", Detail: "go test ./...",
		})
		fmt.Println("    [tool: shell go test]")
		time.Sleep(step)

		// 16) 95%
		emit(events, transport.Progress, plan(tasks, 95, "Tests passing"))
		fmt.Println("    [plan 95%]")
		time.Sleep(step)

		// 17) All done, 100%
		tasks = []transport.PlanTask{
			{Title: "Analyze existing code", Status: transport.TaskCompleted},
			{Title: "Implement new handler", Status: transport.TaskCompleted},
			{Title: "Write unit tests", Status: transport.TaskCompleted},
			{Title: "Update configuration", Status: transport.TaskCompleted},
			{Title: "Run linter & build", Status: transport.TaskCompleted},
		}
		emit(events, transport.Progress, plan(tasks, 100, "All tasks completed"))
		fmt.Println("    [plan 100%]")
		time.Sleep(step)

		// 18) Message: info
		emit(events, transport.Message, transport.MessageData{
			Text: "3 files changed, 247 insertions, 12 deletions", Level: transport.LevelInfo,
		})
		fmt.Println("    [message: info]")
		time.Sleep(step)

		// 19) Final response
		finalMD := "## Implementation Complete\n\n" +
			"Created a new HTTP handler with full test coverage.\n\n" +
			"**Files changed:**\n" +
			"- `internal/api/handler.go` — new endpoint\n" +
			"- `internal/api/handler_test.go` — 8 test cases\n" +
			"- `config.yaml` — added route mapping\n\n" +
			"```go\nfunc (s *Server) HandleCreate(w http.ResponseWriter, r *http.Request) {\n" +
			"    var req CreateRequest\n" +
			"    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {\n" +
			"        http.Error(w, err.Error(), http.StatusBadRequest)\n" +
			"        return\n" +
			"    }\n" +
			"    // ... validation & persistence ...\n" +
			"    w.WriteHeader(http.StatusCreated)\n" +
			"}\n```\n\n" +
			"All **8 tests** passing. Build clean, no linter warnings. ✅"

		emit(events, transport.LLMStream, transport.LLMStreamData{Text: finalMD})
		fmt.Println("    [final text]")
		time.Sleep(300 * time.Millisecond)
		emit(events, transport.Done, transport.DoneData{})
	}()

	reply.ConsumeEvents(ctx, events, nil)
	fmt.Println("    complete")
}
