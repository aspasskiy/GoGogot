package core

import (
	"context"
	"fmt"
	"gogogot/internal/channel"
	"gogogot/internal/core/agent"
	"gogogot/internal/core/agent/hook"
	"gogogot/internal/core/episode"
	"gogogot/internal/core/prompt"
	"gogogot/internal/core/transport"
	"gogogot/internal/infra/config"
	"gogogot/internal/infra/scheduler"
	"gogogot/internal/llm"
	"gogogot/internal/llm/types"
	"gogogot/internal/tools"
	"gogogot/internal/tools/store"
	"gogogot/internal/tools/system"
	"sync"

	"github.com/rs/zerolog/log"
)

type activeSession struct {
	cancel     context.CancelFunc
	replyInbox chan string // unbuffered; used for ask_user responses
}

type Engine struct {
	ch        channel.Channel
	agent     *agent.Agent
	store     *store.Store
	episodes  *episode.Manager
	scheduler *scheduler.Scheduler
	registry  *tools.Registry

	mu       sync.Mutex
	sessions map[string]*activeSession
}

func New(cfg *config.Config, ch channel.Channel) (*Engine, error) {

	st, err := store.New(cfg.DataDir)
	if err != nil {
		return nil, fmt.Errorf("init store: %w", err)
	}

	provider, err := resolveProvider(cfg)
	if err != nil {
		return nil, err
	}

	sched := scheduler.New(cfg.DataDir, nil, st.LoadTimezone())

	extra := append(transport.ChannelTools(),
		system.ScheduleTools(sched)...,
	)
	extra = append(extra, st.IdentityTools(sched.SetLocation)...)

	client := llm.NewClient(*provider, nil)
	epMgr := episode.NewManager(st, client)

	reg := tools.NewRegistry(st, cfg.BraveAPIKey, epMgr.SearchRelevant, extra...)

	client.SetTools(reg.Definitions())

	transportName := ch.Name()
	modelLabel := provider.Label
	agentCfg := agent.Config{
		PromptLoader: func() prompt.PromptContext {
			skills, _ := store.LoadSkills(st.SkillsDir())
			return prompt.PromptContext{
				TransportName: transportName,
				ModelLabel:    modelLabel,
				Soul:          st.ReadSoul(),
				User:          st.ReadUser(),
				SkillsBlock:   store.FormatSkillsForPrompt(skills),
				Timezone:      st.LoadTimezone(),
			}
		},
		MaxTokens:  cfg.MaxTokens,
		Compaction: hook.DefaultCompactionConfig(),
	}

	eng := &Engine{
		ch:        ch,
		store:     st,
		episodes:  epMgr,
		scheduler: sched,
		registry:  reg,
		sessions:  make(map[string]*activeSession),
	}
	eng.agent = agent.New(client, agentCfg, reg)

	ownerSessionID, ownerReply := ch.OwnerSession()
	sched.SetExecutor(func(ctx context.Context, taskID, command, skill string) (string, error) {
		return eng.RunScheduledTask(ctx, ownerSessionID, ownerReply, taskID, command, skill)
	})

	return eng, nil
}

func (e *Engine) Run(ctx context.Context) error {
	if err := e.scheduler.Start(); err != nil {
		return fmt.Errorf("start scheduler: %w", err)
	}
	defer e.scheduler.Stop()

	return e.ch.Run(ctx, e.handleMessage)
}

func resolveProvider(cfg *config.Config) (*llm.Provider, error) {
	if cfg.Provider == "" {
		return nil, fmt.Errorf("GOGOGOT_PROVIDER is required — set to 'anthropic', 'openai', or 'openrouter'")
	}
	if cfg.Model == "" {
		return nil, fmt.Errorf("GOGOGOT_MODEL is required — use an exact model ID (e.g. claude-sonnet-4-6, gpt-4o) or an OpenRouter slug (vendor/model)")
	}
	return llm.ResolveProvider(cfg.Model, cfg.Provider)
}

func (e *Engine) Channel() channel.Channel {
	return e.ch
}

func (e *Engine) handleMessage(ctx context.Context, msg channel.Message) {
	if msg.Command != nil {
		e.handleCommand(ctx, msg)
		return
	}

	e.mu.Lock()
	sess, busy := e.sessions[msg.SessionID]
	e.mu.Unlock()

	if busy {
		select {
		case sess.replyInbox <- msg.Text:
			return
		default:
		}
		_ = msg.Reply.SendText(ctx, "Still working on the previous task, please wait...")
		return
	}

	if msg.Text == "" && len(msg.Attachments) == 0 {
		return
	}

	log.Info().
		Str("session", msg.SessionID).
		Int("text_len", len(msg.Text)).
		Int("attachments", len(msg.Attachments)).
		Msg("engine: incoming message")

	go e.runAgent(ctx, msg)
}

func (e *Engine) handleCommand(ctx context.Context, msg channel.Message) {
	cmd := msg.Command
	switch cmd.Name {
	case channel.CmdNewEpisode:
		cmd.Result.Error = e.episodes.Reset(ctx, msg.SessionID)
	case channel.CmdStop:
		e.stopAgent(msg.SessionID, cmd)
	case channel.CmdHistory:
		episodes, err := e.store.ListEpisodes()
		if err != nil {
			cmd.Result.Error = err
		} else {
			cmd.Result.Payload = episodes
		}
	case channel.CmdMemory:
		files, err := e.store.ListMemory()
		if err != nil {
			cmd.Result.Error = err
		} else {
			cmd.Result.Payload = files
		}
	}
}

func (e *Engine) runAgent(ctx context.Context, msg channel.Message) {
	reply := msg.Reply
	sessionID := msg.SessionID

	agentCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	sess := &activeSession{
		cancel:     cancel,
		replyInbox: make(chan string),
	}
	e.mu.Lock()
	e.sessions[sessionID] = sess
	e.mu.Unlock()
	defer func() {
		e.mu.Lock()
		delete(e.sessions, sessionID)
		e.mu.Unlock()
	}()

	ep, err := e.episodes.Resolve(agentCtx, sessionID, msg.Text)
	if err != nil {
		log.Error().Err(err).Msg("engine: failed to resolve episode")
		_ = reply.SendText(ctx, "Error: "+err.Error())
		return
	}

	agentCtx = transport.WithReplier(agentCtx, reply)

	blocks, cleanup := transport.ProcessAttachments(ep.ID, msg.Text, msg.Attachments)
	defer cleanup()

	bus, recv := transport.NewBus(64)
	go func() {
		defer bus.Close()
		if err := e.agent.Run(agentCtx, ep, blocks, bus); err != nil {
			log.Error().Err(err).Str("session", sessionID).Msg("engine: agent run failed")
		}
	}()

	finalText := reply.ConsumeEvents(agentCtx, recv, sess.replyInbox)
	if finalText != "" {
		_ = reply.SendText(ctx, finalText)
	}
}

func (e *Engine) stopAgent(sessionID string, cmd *channel.Command) {
	e.mu.Lock()
	sess, running := e.sessions[sessionID]
	e.mu.Unlock()

	if !running {
		cmd.Result.Data = map[string]string{"text": "Nothing to cancel."}
		return
	}

	sess.cancel()
	cmd.Result.Data = map[string]string{"text": "⏹ Stopping..."}
}

// RunScheduledTask executes a scheduled task in the active episode for the
// given session. It runs synchronously and returns the agent's text output.
// If the agent is already busy on this session, it returns an error so the
// scheduler can apply backoff and retry later.
func (e *Engine) RunScheduledTask(ctx context.Context, sessionID string, reply transport.Replier, taskID, command, skill string) (string, error) {
	e.mu.Lock()
	_, busy := e.sessions[sessionID]
	e.mu.Unlock()
	if busy {
		return "", fmt.Errorf("agent busy on session %s, will retry", sessionID)
	}

	agentCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	agentCtx = transport.WithReplier(agentCtx, reply)

	sess := &activeSession{cancel: cancel, replyInbox: make(chan string)}
	e.mu.Lock()
	e.sessions[sessionID] = sess
	e.mu.Unlock()
	defer func() {
		e.mu.Lock()
		delete(e.sessions, sessionID)
		e.mu.Unlock()
	}()

	ep, err := e.episodes.Resolve(agentCtx, sessionID, command)
	if err != nil {
		return "", fmt.Errorf("resolve episode: %w", err)
	}

	promptText := prompt.ScheduledTaskPrompt(taskID, command, skill)
	blocks := []types.ContentBlock{types.TextBlock(promptText)}

	bus, recv := transport.NewBus(64)
	var runErr error
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer bus.Close()
		runErr = e.agent.Run(agentCtx, ep, blocks, bus)
	}()

	var finalText string
	for ev := range recv {
		if ev.Kind == transport.LLMStream {
			if d, ok := ev.Data.(transport.LLMStreamData); ok {
				finalText = d.Text
			}
		}
	}
	<-done

	if runErr != nil {
		return "", runErr
	}

	if finalText != "" {
		_ = reply.SendText(ctx, finalText)
	}

	return finalText, nil
}
