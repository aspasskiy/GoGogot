package agent

import (
	"context"
	"gogogot/internal/core/agent/hook"
	"gogogot/internal/core/prompt"
	"gogogot/internal/core/transport"
	"gogogot/internal/llm"
	llmTypes "gogogot/internal/llm/types"
	"gogogot/internal/tools"
	"gogogot/internal/tools/system"
	toolTypes "gogogot/internal/tools/types"
)

type Config struct {
	PromptLoader   func() prompt.PromptContext
	Model          string
	MaxTokens      int
	Tools          []string
	Compaction hook.CompactionConfig
}

type Agent struct {
	client       llm.LLM
	config       Config
	registry     *tools.Registry
	localTools   map[string]toolTypes.Tool
	taskPlan     *system.TaskPlan
	loopDetector *hook.LoopDetector
	beforeHooks  []hook.BeforeIterationFunc
	afterHooks   []hook.AfterIterationFunc
	bus          *transport.Bus
}

func New(client llm.LLM, config Config, registry *tools.Registry) *Agent {
	tp := system.NewTaskPlan()
	tpTool := system.TaskPlanTool(tp)

	a := &Agent{
		client:   client,
		config:   config,
		registry: registry,
		taskPlan: tp,
		localTools: map[string]toolTypes.Tool{
			tpTool.Name:       tpTool,
			"report_status":   reportStatusTool(),
			"send_message":    sendMessageTool(),
			"ask_user":        askUserTool(),
		},
	}

	a.loopDetector = hook.NewLoopDetector(0)
	a.AddBeforeHook(hook.CompactionBeforeIteration(config.Compaction, a.summarize))
	a.AddBeforeHook(hook.LoggingBeforeIteration())
	a.AddAfterHook(hook.LoggingAfterIteration(
		client.InputPricePerM(), client.OutputPricePerM(),
	))
	a.AddAfterHook(hook.UsageAfterIteration(
		client.InputPricePerM(), client.OutputPricePerM(),
	))

	return a
}

func (a *Agent) AddBeforeHook(fn hook.BeforeIterationFunc) {
	a.beforeHooks = append(a.beforeHooks, fn)
}

func (a *Agent) AddAfterHook(fn hook.AfterIterationFunc) {
	a.afterHooks = append(a.afterHooks, fn)
}

func (a *Agent) ModelLabel() string {
	return a.client.ModelLabel()
}

func (a *Agent) summarize(ctx context.Context, prompt string) (string, error) {
	msgs := []llmTypes.Message{
		llmTypes.NewUserMessage(llmTypes.TextBlock(prompt)),
	}
	resp, err := a.client.Call(ctx, msgs, llm.CallOptions{
		System:  a.config.Compaction.SummaryPrompt,
		NoTools: true,
	})
	if err != nil {
		return "", err
	}
	return llmTypes.ExtractText(resp.Content), nil
}

func (a *Agent) runBeforeHooks(ctx context.Context, ic *hook.IterationContext) {
	for _, h := range a.beforeHooks {
		h(ctx, ic)
	}
}

func (a *Agent) runAfterHooks(ctx context.Context, ic *hook.IterationContext, result *hook.IterationResult) {
	for _, h := range a.afterHooks {
		h(ctx, ic, result)
	}
}

func (a *Agent) localToolDefs() []llmTypes.ToolDef {
	defs := make([]llmTypes.ToolDef, 0, len(a.localTools))
	for _, t := range a.localTools {
		defs = append(defs, llmTypes.ToolDef{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  t.Parameters,
			Required:    t.Required,
		})
	}
	return defs
}

func (a *Agent) executeLocal(ctx context.Context, name string, input map[string]any) (toolTypes.Result, bool) {
	t, ok := a.localTools[name]
	if !ok {
		return toolTypes.Result{}, false
	}
	return t.Handler(ctx, input), true
}

func (a *Agent) executeTool(ctx context.Context, name string, input map[string]any) toolTypes.Result {
	if result, handled := a.executeLocal(ctx, name, input); handled {
		return result
	}
	return a.registry.Execute(ctx, name, input)
}

func reportStatusTool() toolTypes.Tool {
	return toolTypes.Tool{
		Name:        "report_status",
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
		Handler: func(_ context.Context, _ map[string]any) toolTypes.Result {
			return toolTypes.Result{Output: "status updated"}
		},
	}
}

func sendMessageTool() toolTypes.Tool {
	return toolTypes.Tool{
		Name:        "send_message",
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
		Handler: func(_ context.Context, _ map[string]any) toolTypes.Result {
			return toolTypes.Result{Output: "message sent"}
		},
	}
}

func askUserTool() toolTypes.Tool {
	return toolTypes.Tool{
		Name:        "ask_user",
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
		Handler: func(_ context.Context, _ map[string]any) toolTypes.Result {
			return toolTypes.Result{Output: "ok"}
		},
	}
}
