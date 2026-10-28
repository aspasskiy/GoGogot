package core

import (
	"context"
	"fmt"
	"sync"

	"github.com/aspasskiy/gogogot/internal/channel"
	"github.com/aspasskiy/gogogot/internal/core/agent"
	"github.com/aspasskiy/gogogot/internal/core/chat"
	"github.com/aspasskiy/gogogot/internal/core/prompt"
	"github.com/aspasskiy/gogogot/internal/domain"
	"github.com/aspasskiy/gogogot/internal/scheduler"
	"github.com/aspasskiy/gogogot/internal/store"
	"github.com/aspasskiy/gogogot/internal/transport"

	"github.com/rs/zerolog/log"
)

type activeRun struct {
	cancel     context.CancelFunc
	replyInbox chan string // unbuffered; used for ask_user responses (nil for scheduled runs)
}

type Engine struct {
	ch        channel.Channel
	agent     *agent.Agent
	store     store.Store
	chats     *chat.Manager
	scheduler *scheduler.Scheduler

	// slot holds a token while a run is in progress: at most one agent run
	// (user message or scheduled task) is active at a time.
	slot chan struct{}

	mu     sync.Mutex
	active *activeRun
}

type Params struct {
	Channel   channel.Channel
	Store     store.Store
	Agent     *agent.Agent
	Chats     *chat.Manager
	Scheduler *scheduler.Scheduler
}

func New(p Params) *Engine {
	eng := &Engine{
		ch:        p.Channel,
		store:     p.Store,
		agent:     p.Agent,
		chats:     p.Chats,
		scheduler: p.Scheduler,
		slot:      make(chan struct{}, 1),
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

func (e *Engine) handleMessage(ctx context.Context, msg channel.Message) {
	if msg.Command != nil {
		e.handleCommand(ctx, msg)
		return
	}

	if run := e.current(); run != nil {
		select {
		case run.replyInbox <- msg.Text:
			return
		default:
		}
		_ = msg.Reply.SendText(ctx, "Still working on the previous task, please wait...")
		return
	}

	if msg.Text == "" && len(msg.Attachments) == 0 {
		return
	}

	agentCtx, cancel := context.WithCancel(ctx)
	run := &activeRun{cancel: cancel, replyInbox: make(chan string)}
	if !e.tryAcquire(run) {
		cancel()
		_ = msg.Reply.SendText(ctx, "Still working on the previous task, please wait...")
		return
	}

	log.Info().
		Int("text_len", len(msg.Text)).
		Int("attachments", len(msg.Attachments)).
		Msg("engine: incoming message")

	go func() {
		defer e.release()
		defer cancel()
		e.runAgent(ctx, agentCtx, run, msg)
	}()
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

// tryAcquire claims the run slot without waiting. It returns false when
// another run is in progress.
func (e *Engine) tryAcquire(run *activeRun) bool {
	select {
	case e.slot <- struct{}{}:
	default:
		return false
	}
	e.setActive(run)
	return true
}

// acquire claims the run slot, waiting until it is free or ctx is done.
func (e *Engine) acquire(ctx context.Context, run *activeRun) error {
	select {
	case e.slot <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	e.setActive(run)
	return nil
}

// release frees the run slot taken by tryAcquire or acquire.
func (e *Engine) release() {
	e.setActive(nil)
	<-e.slot
}

func (e *Engine) setActive(run *activeRun) {
	e.mu.Lock()
	e.active = run
	e.mu.Unlock()
}

func (e *Engine) current() *activeRun {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.active
}

// runAgent executes one user-initiated run. ctx is the channel context (used
// for replies that must outlive a /stop); agentCtx is cancelled by /stop.
func (e *Engine) runAgent(ctx, agentCtx context.Context, run *activeRun, msg channel.Message) {
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
	run := e.current()
	if run == nil {
		cmd.Result.Data = map[string]string{"text": "Nothing to cancel."}
		return
	}

	run.cancel()
	cmd.Result.Data = map[string]string{"text": "⏹ Stopping..."}
}

// RunScheduledTask executes a scheduled task in the active chat.
// It runs synchronously and returns the agent's text output.
// If another run is in progress it waits for it to finish; ctx (the
// scheduler's task timeout) bounds the wait and the run together. A task that
// never got the slot returns scheduler.ErrBusy.
func (e *Engine) RunScheduledTask(ctx context.Context, reply transport.Replier, taskID, command, skill string) (string, error) {
	agentCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	agentCtx = transport.WithReplier(agentCtx, reply)

	run := &activeRun{cancel: cancel}
	if err := e.acquire(ctx, run); err != nil {
		return "", fmt.Errorf("%w: %v", scheduler.ErrBusy, err)
	}
	defer e.release()

	bus, recv := transport.NewBus(64)

	ch, err := e.chats.Resolve()
	if err != nil {
		bus.Close()
		return "", fmt.Errorf("resolve chat: %w", err)
	}

	promptText := prompt.ScheduledTaskPrompt(taskID, command, skill)
	blocks := []domain.ContentBlock{domain.TextBlock(promptText)}

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
		switch ev := ev.(type) {
		case transport.DoneEvent:
			finalText = ev.Text
		case transport.AskEvent:
			// Nobody is watching a scheduled run: answer right away instead of
			// leaving the agent blocked until the task timeout.
			ev.ReplyCh <- "(no interactive input available in a scheduled task — decide on your own)"
		}
	}
	<-done

	if runErr != nil {
		return "", runErr
	}

	if finalText != "" {
		_ = reply.SendText(ctx, finalText)
	}

	e.chats.SummarizeIfNeeded(context.WithoutCancel(ctx), ch)

	return finalText, nil
}
