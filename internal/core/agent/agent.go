package agent

import (
	"context"
	"gogogot/internal/core/agent/event"
	"gogogot/internal/core/agent/hook"
	"gogogot/internal/core/prompt"
	"gogogot/internal/llm"
	llmTypes "gogogot/internal/llm/types"
	"gogogot/internal/tools"
	"gogogot/internal/tools/store"
	"gogogot/internal/tools/system"
	toolTypes "gogogot/internal/tools/types"
)

type Config struct {
	PromptLoader   func() prompt.PromptContext
	Model          string
	MaxTokens      int
	Tools          []string
	Compaction store.CompactionConfig
}

type Agent struct {
	client       llm.LLM
	config       Config
	registry     *tools.Registry
	localTools   map[string]toolTypes.Tool
	loopDetector *hook.LoopDetector
	beforeHooks  []hook.BeforeIterationFunc
	afterHooks   []hook.AfterIterationFunc
	bus          *event.Bus
}

func New(client llm.LLM, config Config, registry *tools.Registry) *Agent {
	tp := system.NewTaskPlan()
	tpTool := system.TaskPlanTool(tp)

	a := &Agent{
		client:   client,
		config:   config,
		registry: registry,
		localTools: map[string]toolTypes.Tool{
			tpTool.Name: tpTool,
		},
	}

	a.loopDetector = hook.NewLoopDetector(0)
	a.AddBeforeHook(hook.CompactionBeforeIteration(config.Compaction, a.summarize))
	a.AddBeforeHook(hook.LoggingBeforeIteration())
	a.AddAfterHook(hook.LoggingAfterIteration())
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
