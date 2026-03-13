package core

import (
	"context"
	"fmt"
	"gogogot/internal/channel"
	"gogogot/internal/core/agent"
	"gogogot/internal/core/episode"
	"gogogot/internal/core/prompt"
	"gogogot/internal/core/transport"
	"gogogot/internal/infra/scheduler"
	"gogogot/internal/llm/types"
	"gogogot/internal/tools"
	"gogogot/internal/tools/store"
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
	store     store.Store
	episodes  *episode.Manager
	scheduler *scheduler.Scheduler
	registry  *tools.Registry

	mu     sync.Mutex
	active *activeSession
}

type Params struct {
	Channel   channel.Channel
	Store     store.Store
	Agent     *agent.Agent
	Episodes  *episode.Manager
	Scheduler *scheduler.Scheduler
	Registry  *tools.Registry
}

func New(p Params) *Engine {
	eng := &Engine{
		ch:        p.Channel,
		store:     p.Store,
		agent:     p.Agent,
		episodes:  p.Episodes,
		scheduler: p.Scheduler,
		registry:  p.Registry,
	}

	ownerReply := p.Channel.OwnerReplier()
	p.Scheduler.SetExecutor(func(ctx context.Context, taskID, command, skill string) (string, error) {
		return eng.RunScheduledTask(ctx, ownerReply, taskID, command, skill)
	})

	return eng
}

func (e *Engine) Run(ctx context.Context) error {
	if err := e.scheduler.Start(); err != nil {
		return fmt.Errorf("start scheduler: %w", err)
	}
	defer e.scheduler.Stop()

	return e.ch.Run(ctx, e.handleMessage)
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
	sess := e.active
	e.mu.Unlock()

	if sess != nil {
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
		Int("text_len", len(msg.Text)).
		Int("attachments", len(msg.Attachments)).
		Msg("engine: incoming message")

	go e.runAgent(ctx, msg)
}

func (e *Engine) handleCommand(ctx context.Context, msg channel.Message) {
	cmd := msg.Command
	switch cmd.Name {
	case channel.CmdNewEpisode:
		cmd.Result.Error = e.episodes.Reset(ctx)
	case channel.CmdStop:
		e.stopAgent(cmd)
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

// setActive marks the engine as busy and returns a release function.
// Caller must defer the release.
func (e *Engine) setActive(sess *activeSession) func() {
	e.mu.Lock()
	e.active = sess
	e.mu.Unlock()
	return func() {
		e.mu.Lock()
		e.active = nil
		e.mu.Unlock()
	}
}

func (e *Engine) runAgent(ctx context.Context, msg channel.Message) {
	reply := msg.Reply

	agentCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	sess := &activeSession{
		cancel:     cancel,
		replyInbox: make(chan string),
	}
	defer e.setActive(sess)()

	ep, err := e.episodes.Resolve(agentCtx, msg.Text)
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
			log.Error().Err(err).Msg("engine: agent run failed")
		}
	}()

	finalText := reply.ConsumeEvents(agentCtx, recv, sess.replyInbox)
	if finalText != "" {
		_ = reply.SendText(ctx, finalText)
	}
}

func (e *Engine) stopAgent(cmd *channel.Command) {
	e.mu.Lock()
	sess := e.active
	e.mu.Unlock()

	if sess == nil {
		cmd.Result.Data = map[string]string{"text": "Nothing to cancel."}
		return
	}

	sess.cancel()
	cmd.Result.Data = map[string]string{"text": "⏹ Stopping..."}
}

// RunScheduledTask executes a scheduled task in the active episode.
// It runs synchronously and returns the agent's text output.
// If the agent is already busy, it returns an error so the scheduler can
// apply backoff and retry later.
func (e *Engine) RunScheduledTask(ctx context.Context, reply transport.Replier, taskID, command, skill string) (string, error) {
	e.mu.Lock()
	busy := e.active != nil
	e.mu.Unlock()
	if busy {
		return "", fmt.Errorf("agent busy, will retry")
	}

	agentCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	agentCtx = transport.WithReplier(agentCtx, reply)

	sess := &activeSession{cancel: cancel, replyInbox: make(chan string)}
	defer e.setActive(sess)()

	ep, err := e.episodes.Resolve(agentCtx, command)
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
