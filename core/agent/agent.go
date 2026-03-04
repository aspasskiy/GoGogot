package agent

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"gogogot/core/prompt"
	"gogogot/core/store"
	"gogogot/infra/llm"
	"gogogot/infra/llm/types"
	"gogogot/tools"
	"gogogot/tools/system"

	"github.com/rs/zerolog/log"
)

type AgentConfig struct {
	PromptCtx      prompt.PromptContext
	Model          string
	MaxTokens      int
	Tools          []string
	Compaction     CompactionConfig
	EvalIterations int
}

type Agent struct {
	client      llm.LLM
	Chat        *store.Chat
	Events      chan Event
	config      AgentConfig
	session     *Session
	registry    *system.Registry
	localTools  map[string]tools.Tool
	beforeHooks []BeforeToolCallFunc
	afterHooks  []AfterToolCallFunc
}

func New(client llm.LLM, chat *store.Chat, config AgentConfig, registry *system.Registry) *Agent {
	tp := system.NewTaskPlan()
	tpTool := system.TaskPlanTool(tp)

	a := &Agent{
		client:   client,
		Chat:     chat,
		Events:   make(chan Event, 64),
		config:   config,
		session:  NewSession(chat.ID, ""),
		registry: registry,
		localTools: map[string]tools.Tool{
			tpTool.Name: tpTool,
		},
	}

	ld := NewLoopDetector(0)
	a.AddBeforeHook(ld.BeforeHook())

	return a
}

// localToolDefs returns LLM tool definitions for session-scoped tools.
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

// executeLocal tries to dispatch a tool call to a session-scoped tool.
// Returns the result and true if handled, or zero value and false otherwise.
func (a *Agent) executeLocal(ctx context.Context, name string, input map[string]any) (tools.Result, bool) {
	t, ok := a.localTools[name]
	if !ok {
		return tools.Result{}, false
	}
	return t.Handler(ctx, input), true
}

func (a *Agent) AddBeforeHook(fn BeforeToolCallFunc) {
	a.beforeHooks = append(a.beforeHooks, fn)
}

func (a *Agent) AddAfterHook(fn AfterToolCallFunc) {
	a.afterHooks = append(a.afterHooks, fn)
}

func (a *Agent) emit(kind EventKind, data any) {
	select {
	case a.Events <- Event{
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
	a.session = NewSession(chat.ID, "")
}

func uniqueName(name string, counts map[string]int) string {
	counts[name]++
	if counts[name] == 1 {
		return name
	}
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	return fmt.Sprintf("%s_%d%s", base, counts[name], ext)
}
