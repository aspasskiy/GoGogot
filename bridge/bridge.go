package bridge

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"gogogot/agent"
	"gogogot/event"
	"gogogot/store"
	"gogogot/llm"
	"gogogot/llm/types"
	"gogogot/transport"
	"gogogot/tools"

	"github.com/rs/zerolog/log"
)

type Bridge struct {
	transport transport.Transport
	llmClient llm.LLM
	agentCfg  agent.AgentConfig
	registry  *tools.Registry

	mu      sync.Mutex
	agents  map[string]*agent.Agent
	cancels map[string]context.CancelFunc
}

func New(t transport.Transport, client llm.LLM, cfg agent.AgentConfig, reg *tools.Registry) *Bridge {
	return &Bridge{
		transport: t,
		llmClient: client,
		agentCfg:  cfg,
		registry:  reg,
		agents:  make(map[string]*agent.Agent),
		cancels: make(map[string]context.CancelFunc),
	}
}

func (b *Bridge) Run(ctx context.Context) error {
	return b.transport.Run(ctx, b.handleMessage)
}

func (b *Bridge) Transport() transport.Transport {
	return b.transport
}

func (b *Bridge) handleMessage(ctx context.Context, msg transport.Message) {
	if msg.Command != nil {
		b.handleCommand(ctx, msg)
		return
	}

	channelID := msg.ChannelID

	b.mu.Lock()
	_, busy := b.cancels[channelID]
	b.mu.Unlock()

	if busy {
		_ = b.transport.SendText(ctx, channelID, "Still working on the previous task, please wait...")
		return
	}

	if msg.Text == "" && len(msg.Attachments) == 0 {
		return
	}

	log.Info().
		Str("channel", channelID).
		Int("text_len", len(msg.Text)).
		Int("attachments", len(msg.Attachments)).
		Msg("bridge: incoming message")

	go b.runAgent(ctx, channelID, msg)
}

func (b *Bridge) handleCommand(ctx context.Context, msg transport.Message) {
	cmd := msg.Command
	switch cmd.Name {
	case transport.CmdNewChat:
		cmd.Result.Error = b.resetChat(msg.ChannelID)
	case transport.CmdSwitchChat:
		title, err := b.switchChat(msg.ChannelID, cmd.Args["chat_id"])
		cmd.Result.Error = err
		if err == nil {
			cmd.Result.Data = map[string]string{"title": title}
		}
	case transport.CmdStop:
		b.stopAgent(ctx, msg.ChannelID)
	}
}

func (b *Bridge) runAgent(ctx context.Context, channelID string, msg transport.Message) {
	agentCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	b.mu.Lock()
	b.cancels[channelID] = cancel
	b.mu.Unlock()
	defer func() {
		b.mu.Lock()
		delete(b.cancels, channelID)
		b.mu.Unlock()
	}()

	a, err := b.getOrCreateAgent(channelID)
	if err != nil {
		log.Error().Err(err).Msg("bridge: failed to get agent")
		_ = b.transport.SendText(ctx, channelID, "Error: "+err.Error())
		return
	}

	if tn, ok := b.transport.(transport.TypingNotifier); ok {
		_ = tn.SendTyping(ctx, channelID)
	}

	var statusID string
	if su, ok := b.transport.(transport.StatusUpdater); ok {
		statusID, _ = su.SendStatus(ctx, channelID, "Working on it...")
	}

	agentCtx = transport.WithTransport(agentCtx, b.transport, channelID)

	blocks, cleanup := processAttachments(a.Chat.ID, msg.Text, msg.Attachments)
	defer cleanup()

	a.Events = make(chan event.Event, 64)
	events := a.Events

	go func() {
		defer close(events)
		if err := a.Run(agentCtx, blocks); err != nil {
			log.Error().Err(err).Str("channel", channelID).Msg("bridge: agent run failed")
		}
	}()

	b.consumeEvents(agentCtx, channelID, events, statusID)
}

func (b *Bridge) stopAgent(ctx context.Context, channelID string) {
	b.mu.Lock()
	cancel, running := b.cancels[channelID]
	b.mu.Unlock()

	if !running {
		_ = b.transport.SendText(ctx, channelID, "Nothing to cancel.")
		return
	}

	cancel()
	_ = b.transport.SendText(ctx, channelID, "⏹ Stopping...")
}

// RunScheduledTask executes a scheduled task in the active chat for the given
// channel. It runs synchronously and returns the agent's text output.
// If the agent is already busy on this channel, it returns an error so the
// scheduler can apply backoff and retry later.
func (b *Bridge) RunScheduledTask(ctx context.Context, channelID, taskID, command, skill string) (string, error) {
	b.mu.Lock()
	_, busy := b.cancels[channelID]
	b.mu.Unlock()
	if busy {
		return "", fmt.Errorf("agent busy on channel %s, will retry", channelID)
	}

	agentCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	b.mu.Lock()
	b.cancels[channelID] = cancel
	b.mu.Unlock()
	defer func() {
		b.mu.Lock()
		delete(b.cancels, channelID)
		b.mu.Unlock()
	}()

	a, err := b.getOrCreateAgent(channelID)
	if err != nil {
		return "", fmt.Errorf("get agent: %w", err)
	}

	prompt := buildScheduledPrompt(taskID, command, skill)
	blocks := []types.ContentBlock{types.TextBlock(prompt)}

	a.Events = make(chan event.Event, 64)
	events := a.Events

	var runErr error
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer close(events)
		runErr = a.Run(agentCtx, blocks)
	}()

	var finalText string
	for ev := range events {
		if ev.Kind == event.LLMStream {
			if text, ok := ev.Data.(map[string]any)["text"].(string); ok {
				finalText = text
			}
		}
	}
	<-done

	if runErr != nil {
		return "", runErr
	}

	if finalText != "" {
		_ = b.transport.SendText(ctx, channelID, finalText)
	}

	return finalText, nil
}

func buildScheduledPrompt(taskID, command, skill string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[Scheduled Task: %s]\n", taskID)
	b.WriteString("You woke up from a scheduled trigger. Execute the following instruction " +
		"using your tools, memory, and skills. Do not write standalone scripts.\n\n")
	fmt.Fprintf(&b, "Instruction: %s", command)
	if skill != "" {
		fmt.Fprintf(&b, "\nSkill: Read skill %q with skill_read and follow its instructions.", skill)
	}
	return b.String()
}

func (b *Bridge) consumeEvents(ctx context.Context, channelID string, events <-chan event.Event, statusID string) {
	var finalText string
	var toolsUsed []string

	for ev := range events {
		switch ev.Kind {
		case event.LLMStream:
			text, _ := ev.Data.(map[string]any)["text"].(string)
			finalText = text

		case event.ToolStart:
			name, _ := ev.Data.(map[string]any)["name"].(string)
			toolsUsed = append(toolsUsed, name)
			log.Debug().Str("name", name).Str("channel", channelID).Msg("bridge: tool running")

			if su, ok := b.transport.(transport.StatusUpdater); ok && statusID != "" {
				_ = su.UpdateStatus(ctx, channelID, statusID, fmt.Sprintf("Running %s...", name))
			}
			if tn, ok := b.transport.(transport.TypingNotifier); ok {
				_ = tn.SendTyping(ctx, channelID)
			}

		case event.Error:
			if ctx.Err() != nil {
				return
			}
			errMap, ok := ev.Data.(map[string]any)
			var errText string
			if ok {
				errText, _ = errMap["error"].(string)
			}
			if su, ok := b.transport.(transport.StatusUpdater); ok && statusID != "" {
				_ = su.UpdateStatus(ctx, channelID, statusID, "Error: "+errText)
			} else {
				_ = b.transport.SendText(ctx, channelID, "Error: "+errText)
			}
			return

		case event.Done:
			cancelled := ctx.Err() != nil
			log.Info().
				Str("channel", channelID).
				Strs("tools_used", toolsUsed).
				Int("response_len", len(finalText)).
				Bool("cancelled", cancelled).
				Msg("bridge: agent done")
			if su, ok := b.transport.(transport.StatusUpdater); ok && statusID != "" {
				_ = su.DeleteStatus(context.Background(), channelID, statusID)
			}
			if !cancelled && finalText != "" {
				_ = b.transport.SendText(ctx, channelID, finalText)
			}
			return
		}
	}
}

func (b *Bridge) getOrCreateAgent(channelID string) (*agent.Agent, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if a, ok := b.agents[channelID]; ok {
		return a, nil
	}

	chat, err := store.LoadOrCreateByExternalID(channelID)
	if err != nil {
		return nil, err
	}

	a := agent.New(b.llmClient, chat, b.agentCfg, b.registry)
	b.agents[channelID] = a
	return a, nil
}

func (b *Bridge) resetChat(channelID string) error {
	b.mu.Lock()
	delete(b.agents, channelID)
	b.mu.Unlock()

	newChat := store.NewChat()
	if err := newChat.Save(); err != nil {
		return err
	}
	if err := store.SetExternalMapping(channelID, newChat.ID); err != nil {
		return err
	}

	a := agent.New(b.llmClient, newChat, b.agentCfg, b.registry)
	b.mu.Lock()
	b.agents[channelID] = a
	b.mu.Unlock()
	return nil
}

func (b *Bridge) switchChat(channelID, sofieID string) (string, error) {
	chat, err := store.LoadChat(sofieID)
	if err != nil {
		return "", err
	}

	if err := store.SetExternalMapping(channelID, chat.ID); err != nil {
		return "", err
	}

	a := agent.New(b.llmClient, chat, b.agentCfg, b.registry)
	b.mu.Lock()
	b.agents[channelID] = a
	b.mu.Unlock()

	title := chat.Title
	if title == "" {
		title = "Untitled"
	}
	return title, nil
}
