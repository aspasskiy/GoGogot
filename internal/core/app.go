package core

import (
	"context"
	"fmt"
	"github.com/aspasskiy/gogogot/internal/channel"
	"github.com/aspasskiy/gogogot/internal/core/agent"
	"github.com/aspasskiy/gogogot/internal/core/chat"
	"github.com/aspasskiy/gogogot/internal/core/prompt"
	"github.com/aspasskiy/gogogot/internal/transport"
	"github.com/aspasskiy/gogogot/internal/scheduler"
	types "github.com/aspasskiy/gogogot/internal/domain"
	"github.com/aspasskiy/gogogot/internal/tools"
	"github.com/aspasskiy/gogogot/internal/store"
	"sync"

	"github.com/rs/zerolog/log"
)

type activeRun struct {
	cancel     context.CancelFunc
	replyInbox chan string // unbuffered; used for ask_user responses
}

type Engine struct {
	ch        channel.Channel
	agent     *agent.Agent
	store     store.Store
	chats     *chat.Manager
	scheduler *scheduler.Scheduler
	registry  *tools.Registry

	mu     sync.Mutex
	active *activeRun
}

type Params struct {
	Channel   channel.Channel
	Store     store.Store
	Agent     *agent.Agent
	Chats     *chat.Manager
	Scheduler *scheduler.Scheduler
	Registry  *tools.Registry
}

func New(p Params) *Engine {
	eng := &Engine{
		ch:        p.Channel,
		store:     p.Store,
		agent:     p.Agent,
		chats:     p.Chats,
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

	agentCtx, cancel := context.WithCancel(ctx)
	run := &activeRun{
		cancel:     cancel,
		replyInbox: make(chan string),
	}

	// Check-and-claim atomically: if a run is already active, hand the message
	// to it (or report busy) without spawning. Claiming under the same lock hold
	// as the check is what prevents two dispatches from both starting a run.
	release, existing, claimed := e.claim(run)
	if !claimed {
		cancel()
		select {
		case existing.replyInbox <- msg.Text:
			return
		default:
		}
		_ = msg.Reply.SendText(ctx, "Still working on the previous task, please wait...")
		return
	}

	if msg.Text == "" && len(msg.Attachments) == 0 {
		release()
		cancel()
		return
	}

	log.Info().
		Int("text_len", len(msg.Text)).
		Int("attachments", len(msg.Attachments)).
		Msg("engine: incoming message")

	go e.runAgent(ctx, agentCtx, msg, run, release)
}

func (e *Engine) handleCommand(ctx context.Context, msg channel.Message) {
	cmd := msg.Command
	switch cmd.Name {
	case channel.CmdNewChat:
		cmd.Result.Error = e.chats.Reset(ctx)
	case channel.CmdStop:
		e.stopAgent(cmd)
	case channel.CmdHistory:
		chats, err := e.store.ListChats()
		if err != nil {
			cmd.Result.Error = err
		} else {
			cmd.Result.Payload = chats
		}
	case channel.CmdMemory:
		files, err := e.store.ListMemory()
		if err != nil {
			cmd.Result.Error = err
		} else {
			cmd.Result.Payload = files
		}
	case channel.CmdSoul:
		if soul := e.store.ReadSoul(); soul != "" {
			cmd.Result.Data = map[string]string{"text": "🧬 **Soul:**\n\n" + soul}
		}
	case channel.CmdUser:
		if user := e.store.ReadUser(); user != "" {
			cmd.Result.Data = map[string]string{"text": "👤 **User:**\n\n" + user}
		}
	}
}

// claim atomically registers run as the active run if the engine is idle.
// On success it returns a release function (which the caller must defer) and
// claimed=true. If a run is already active it returns that run as existing and
// claimed=false, having claimed nothing. The check and the claim happen under a
// single lock hold, so two callers can never both observe an idle engine.
func (e *Engine) claim(run *activeRun) (release func(), existing *activeRun, claimed bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.active != nil {
		return nil, e.active, false
	}
	e.active = run
	return func() {
		e.mu.Lock()
		e.active = nil
		e.mu.Unlock()
	}, nil, true
}

// runAgent drives one already-claimed run to completion. ctx is the parent
// context (used for sends that must survive cancellation); agentCtx is the
// cancellable run context. release frees the engine's active slot.
func (e *Engine) runAgent(ctx, agentCtx context.Context, msg channel.Message, run *activeRun, release func()) {
	defer release()
	defer run.cancel()

	reply := msg.Reply

	bus, recv := transport.NewBus(64)

	ch, err := e.chats.Resolve()
	if err != nil {
		bus.Close()
		log.Error().Err(err).Msg("engine: failed to resolve chat")
		_ = reply.SendText(ctx, "Error: "+err.Error())
		return
	}

	agentCtx = transport.WithReplier(agentCtx, reply)

	blocks, cleanup := transport.ProcessAttachments(ch.ID, msg.Text, msg.Attachments)
	defer cleanup()

	input := agent.RunInput{
		Messages:   ch.Messages(),
		UserBlocks: blocks,
	}
	sink := newAgentSink(agentCtx, ch, bus)

	go func() {
		defer bus.Close()
		if err := e.agent.Run(agentCtx, input, sink); err != nil {
			log.Error().Err(err).Msg("engine: agent run failed")
		}
	}()

	finalText := reply.ConsumeEvents(agentCtx, recv, run.replyInbox)
	if finalText != "" {
		_ = reply.SendText(ctx, finalText)
	}

	e.chats.SummarizeIfNeeded(context.WithoutCancel(ctx), ch)
}

func (e *Engine) stopAgent(cmd *channel.Command) {
	e.mu.Lock()
	run := e.active
	e.mu.Unlock()

	if run == nil {
		cmd.Result.Data = map[string]string{"text": "Nothing to cancel."}
		return
	}

	run.cancel()
	cmd.Result.Data = map[string]string{"text": "⏹ Stopping..."}
}

// RunScheduledTask executes a scheduled task in the active chat.
// It runs synchronously and returns the agent's text output.
// If the agent is already busy, it returns an error so the scheduler can
// apply backoff and retry later.
func (e *Engine) RunScheduledTask(ctx context.Context, reply transport.Replier, taskID, command, skill string) (string, error) {
	agentCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Atomic check-and-claim: the scheduler fires on its own goroutine, fully
	// parallel to the Telegram worker, so a non-atomic busy check here would let
	// a scheduled task and a chat message both start runs at once.
	run := &activeRun{cancel: cancel, replyInbox: make(chan string)}
	release, _, claimed := e.claim(run)
	if !claimed {
		return "", fmt.Errorf("agent busy, will retry")
	}
	defer release()

	agentCtx = transport.WithReplier(agentCtx, reply)

	bus, recv := transport.NewBus(64)

	ch, err := e.chats.Resolve()
	if err != nil {
		bus.Close()
		return "", fmt.Errorf("resolve chat: %w", err)
	}

	promptText := prompt.ScheduledTaskPrompt(taskID, command, skill)
	blocks := []types.ContentBlock{types.TextBlock(promptText)}

	input := agent.RunInput{
		Messages:   ch.Messages(),
		UserBlocks: blocks,
	}
	sink := newAgentSink(agentCtx, ch, bus)

	var runErr error
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer bus.Close()
		runErr = e.agent.Run(agentCtx, input, sink)
	}()

	// Drain events until the run finishes; the final text is carried explicitly
	// by DoneEvent (not reconstructed from the streaming events).
	var finalText string
	for ev := range recv {
		if d, ok := ev.(transport.DoneEvent); ok {
			finalText = d.Text
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
