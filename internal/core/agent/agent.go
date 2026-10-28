package agent

import (
	"context"
	"sort"

	"github.com/aspasskiy/gogogot/internal/core/agent/hook"
	"github.com/aspasskiy/gogogot/internal/domain"
	"github.com/aspasskiy/gogogot/internal/llm"
	"github.com/aspasskiy/gogogot/internal/tools"
	"github.com/aspasskiy/gogogot/internal/tools/system"
	toolTypes "github.com/aspasskiy/gogogot/internal/tools/types"
	"github.com/aspasskiy/gogogot/internal/transport"
)

// Agent holds the configuration shared by every run: the LLM client, the
// system prompt builder, the tool registry and the hooks. It carries no
// per-run state, so concurrent Run calls are safe.
type Agent struct {
	client       llm.LLM
	instructions func() string
	registry     *tools.Registry
	beforeHooks  []hook.BeforeIterationFunc
	afterHooks   []hook.AfterIterationFunc
}

func New(client llm.LLM, instructions func() string, registry *tools.Registry) *Agent {
	a := &Agent{
		client:       client,
		instructions: instructions,
		registry:     registry,
	}

	a.AddBeforeHook(hook.LoggingBeforeIteration())
	a.AddAfterHook(hook.LoggingAfterIteration(client.Pricing()))
	a.AddAfterHook(hook.UsageAfterIteration(client.Pricing()))

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

// run is the state of a single Agent.Run: the event sink, the loop detector
// and the task plan with the local tools bound to it. A fresh run is created
// for every Run call, so nothing leaks from one user message to the next.
type run struct {
	*Agent
	sink          transport.Sink
	loopDetector  *hook.LoopDetector
	localTools    map[string]toolTypes.Tool
	localToolDefs []domain.ToolDef
}

func (a *Agent) newRun(sink transport.Sink) *run {
	localTools := system.AgentTools(system.NewTaskPlan())

	ltMap := make(map[string]toolTypes.Tool, len(localTools))
	defs := make([]domain.ToolDef, 0, len(localTools))
	for _, t := range localTools {
		ltMap[t.Name] = t
		defs = append(defs, domain.ToolDef{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  t.Parameters,
			Required:    t.Required,
		})
	}
	// Stable order keeps the request prefix identical between calls, which
	// prompt caching depends on.
	sort.Slice(defs, func(i, j int) bool { return defs[i].Name < defs[j].Name })

	return &run{
		Agent:         a,
		sink:          sink,
		loopDetector:  hook.NewLoopDetector(0),
		localTools:    ltMap,
		localToolDefs: defs,
	}
}

// emit sends an event to the run's sink. The sink (the orchestrator) decides
// what to persist and what to forward to the UI.
func (r *run) emit(ev transport.Event) {
	r.sink.Emit(ev)
}

func (r *run) runBeforeHooks(ctx context.Context, ic *hook.IterationContext) {
	for _, h := range r.beforeHooks {
		h(ctx, ic)
	}
}

func (r *run) runAfterHooks(ctx context.Context, ic *hook.IterationContext, result *hook.IterationResult) {
	for _, h := range r.afterHooks {
		h(ctx, ic, result)
	}
}

func (r *run) lookupTool(name string) (toolTypes.Tool, bool) {
	if t, ok := r.localTools[name]; ok {
		return t, true
	}
	return r.registry.Lookup(name)
}

func (r *run) executeTool(ctx context.Context, name string, input map[string]any) toolTypes.Result {
	if t, ok := r.localTools[name]; ok {
		return t.Handler(ctx, input)
	}
	return r.registry.Execute(ctx, name, input)
}
