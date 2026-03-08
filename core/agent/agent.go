package agent

import (
	"context"
	"time"

	"gogogot/core/agent/event"
	"gogogot/core/agent/hook"
	"gogogot/core/agent/prompt"
	"gogogot/core/agent/session"
	"gogogot/llm"
	"gogogot/llm/types"
	"gogogot/store"
	"gogogot/tools"
	"gogogot/tools/system"

	"github.com/rs/zerolog/log"
)

type AgentConfig struct {
	PromptCtx      prompt.PromptContext
	Model          string
	MaxTokens      int
	Tools          []string
	Compaction     session.CompactionConfig
	EvalIterations int
}

type Agent struct {
	client         llm.LLM
	Chat           *store.Chat
	Events         chan event.Event
	config         AgentConfig
	session        *session.Session
	registry       *tools.Registry
	localTools     map[string]tools.Tool
	beforeHooks    []hook.BeforeToolCallFunc
	afterHooks     []hook.AfterToolCallFunc
	beforeLLMHooks []hook.BeforeLLMCallFunc
	afterLLMHooks  []hook.AfterLLMCallFunc
}

func New(client llm.LLM, chat *store.Chat, config AgentConfig, registry *tools.Registry) *Agent {
	tp := system.NewTaskPlan()
	tpTool := system.TaskPlanTool(tp)

	a := &Agent{
		client:   client,
		Chat:     chat,
		Events:   make(chan event.Event, 64),
		config:   config,
		session:  session.NewSession(chat.ID, ""),
		registry: registry,
		localTools: map[string]tools.Tool{
			tpTool.Name: tpTool,
		},
	}

	ld := hook.NewLoopDetector(0)
	a.AddBeforeHook(ld.BeforeHook())
	a.AddBeforeHook(hook.LoggingBeforeHook())
	a.AddAfterHook(hook.LoggingAfterHook())
	a.AddBeforeLLMHook(hook.LLMLoggingBeforeHook())
	a.AddAfterLLMHook(hook.LLMLoggingAfterHook())

	return a
}

func (a *Agent) localToolDefs() []types.ToolDef {
	defs := make([]types.ToolDef, 0, len(a.localTools))
	for _, t := range a.localTools {
		defs = append(defs, types.ToolDef{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  t.Parameters,
			Required:    t.Required,
		})
	}
	return defs
}

func (a *Agent) executeLocal(ctx context.Context, name string, input map[string]any) (tools.Result, bool) {
	t, ok := a.localTools[name]
	if !ok {
		return tools.Result{}, false
	}
	return t.Handler(ctx, input), true
}

func (a *Agent) AddBeforeHook(fn hook.BeforeToolCallFunc) {
	a.beforeHooks = append(a.beforeHooks, fn)
}

func (a *Agent) AddAfterHook(fn hook.AfterToolCallFunc) {
	a.afterHooks = append(a.afterHooks, fn)
}

func (a *Agent) AddBeforeLLMHook(fn hook.BeforeLLMCallFunc) {
	a.beforeLLMHooks = append(a.beforeLLMHooks, fn)
}

func (a *Agent) AddAfterLLMHook(fn hook.AfterLLMCallFunc) {
	a.afterLLMHooks = append(a.afterLLMHooks, fn)
}

func (a *Agent) emit(kind event.Kind, data any) {
	select {
	case a.Events <- event.Event{
		Timestamp: time.Now(),
		Kind:      kind,
		Source:    "core-loop",
		Depth:     0,
		Data:      data,
	}:
	default:
		log.Warn().Any("kind", kind).Msg("agent event dropped — bus full")
	}
}

func (a *Agent) ModelLabel() string {
	return a.client.ModelLabel()
}

func (a *Agent) SetChat(chat *store.Chat) {
	a.Chat = chat
	a.session = session.NewSession(chat.ID, "")
}
