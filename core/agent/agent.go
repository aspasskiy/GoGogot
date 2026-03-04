package agent

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"gogogot/core/prompt"
	"gogogot/core/store"
	"gogogot/infra/llm"
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
	beforeHooks []BeforeToolCallFunc
	afterHooks  []AfterToolCallFunc
}

func New(client llm.LLM, chat *store.Chat, config AgentConfig, registry *system.Registry) *Agent {
	a := &Agent{
		client:   client,
		Chat:     chat,
		Events:   make(chan Event, 64),
		config:   config,
		session:  NewSession(chat.ID, ""),
		registry: registry,
	}

	ld := NewLoopDetector(0)
	a.AddBeforeHook(ld.BeforeHook())

	return a
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
