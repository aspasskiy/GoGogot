package agent

import (
	"context"
	"github.com/aspasskiy/gogogot/internal/core/agent/hook"
	"github.com/aspasskiy/gogogot/internal/core/transport"
	"github.com/aspasskiy/gogogot/internal/llm"
	llmTypes "github.com/aspasskiy/gogogot/internal/llm/types"
	"github.com/aspasskiy/gogogot/internal/tools"
	"github.com/aspasskiy/gogogot/internal/tools/system"
	toolTypes "github.com/aspasskiy/gogogot/internal/tools/types"
)

type Agent struct {
	client       llm.LLM
	instructions func() string
	registry     *tools.Registry
	localTools   map[string]toolTypes.Tool
	taskPlan     *system.TaskPlan
	loopDetector *hook.LoopDetector
	beforeHooks  []hook.BeforeIterationFunc
	afterHooks   []hook.AfterIterationFunc
	bus          *transport.Bus
}

func New(client llm.LLM, instructions func() string, registry *tools.Registry) *Agent {
	tp := system.NewTaskPlan()
	localTools := system.AgentTools(tp)

	ltMap := make(map[string]toolTypes.Tool, len(localTools))
	for _, t := range localTools {
		ltMap[t.Name] = t
	}

	a := &Agent{
		client:       client,
		instructions: instructions,
		registry:     registry,
		taskPlan:     tp,
		localTools:   ltMap,
	}

	a.loopDetector = hook.NewLoopDetector(0)
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

func (a *Agent) lookupTool(name string) (toolTypes.Tool, bool) {
	if t, ok := a.localTools[name]; ok {
		return t, true
	}
	return a.registry.Lookup(name)
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
