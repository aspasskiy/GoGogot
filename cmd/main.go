package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"gogogot/core/agent"
	"gogogot/core/agent/orchestration"
	"gogogot/core/scheduler"
	"gogogot/core/store"
	"gogogot/infra/config"
	"gogogot/infra/llm"
	"gogogot/infra/logger"
	"gogogot/infra/transport/bridge"
	"gogogot/infra/transport/telegram"
	"gogogot/tools/system"

	"github.com/joho/godotenv"
	"github.com/rs/zerolog/log"
)

func main() {
	_ = godotenv.Load()

	modelFlag := flag.String("model", "", "model ID from models.json (default: first available)")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	if *modelFlag != "" {
		cfg.Model = *modelFlag
	}

	if err := logger.Init(cfg.DataDir, cfg.LogLevel); err != nil {
		fmt.Fprintf(os.Stderr, "warning: %v\n", err)
	}
	defer logger.Close()

	store.Init(cfg.DataDir)

	provider, err := selectProvider(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	t, err := buildTransport(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	ownerChannelID := fmt.Sprintf("tg_%d", t.OwnerID())

	sched := scheduler.New(cfg.DataDir, nil)

	allTools := coreTools(cfg.BraveAPIKey, sched)
	allTools = append(allTools, bridge.TransportTools()...)
	reg := system.NewRegistry(allTools)

	client := llm.NewClient(*provider, reg.Definitions())
	agentCfg := agent.AgentConfig{
		SystemPrompt: agent.SystemPrompt(agent.PromptContext{
			TransportName: t.Name(),
			ModelLabel:    provider.Label,
		}),
		MaxTokens:  4096,
		Compaction: agent.DefaultCompaction(),
	}

	executor := buildTaskExecutor(t, client, agentCfg, reg, ownerChannelID)
	sched.SetExecutor(executor)

	if err := sched.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "error starting scheduler: %v\n", err)
		os.Exit(1)
	}
	defer sched.Stop()

	b := bridge.New(t, client, agentCfg, reg)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	fmt.Printf("Sofie is running [%s, %s]. Press Ctrl+C to stop.\n", t.Name(), provider.Label)
	if err := b.Run(ctx); err != nil && ctx.Err() == nil {
		log.Error().Err(err).Msg("bridge run error")
	}
	fmt.Println("Shutting down.")
}

// buildTaskExecutor creates an in-process executor that runs a one-shot agent
// for each scheduled task and sends the result to the owner via Telegram.
func buildTaskExecutor(
	t *telegram.Transport,
	client llm.LLM,
	agentCfg agent.AgentConfig,
	reg *system.Registry,
	ownerChannelID string,
) scheduler.TaskExecutor {
	return func(ctx context.Context, taskID, command string) (string, error) {
		chat := store.NewChat()
		chat.Title = fmt.Sprintf("cron:%s", taskID)
		if err := chat.Save(); err != nil {
			return "", fmt.Errorf("create chat: %w", err)
		}

		a := agent.New(client, chat, agentCfg, reg)
		a.Events = make(chan orchestration.Event, 64)
		events := a.Events

		var runErr error
		done := make(chan struct{})
		go func() {
			defer close(done)
			defer close(events)
			runErr = a.Run(ctx, command)
		}()

		var finalText string
		for ev := range events {
			if ev.Kind == orchestration.EventLLMStream {
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
			prefix := fmt.Sprintf("⏰ [cron:%s]\n\n", taskID)
			if err := t.SendText(ctx, ownerChannelID, prefix+finalText); err != nil {
				log.Error().Err(err).Str("task", taskID).Msg("scheduler: failed to send result to owner")
			}
		}

		return finalText, nil
	}
}

func selectProvider(cfg *config.Config) (*llm.Provider, error) {
	providers, err := llm.LoadProviders(cfg.DataDir)
	if err != nil {
		return nil, err
	}
	if len(providers) == 0 {
		return nil, fmt.Errorf("no LLM providers available — set ANTHROPIC_API_KEY or OPENROUTER_API_KEY")
	}
	if cfg.Model == "" {
		return &providers[0], nil
	}
	for i := range providers {
		if providers[i].ID == cfg.Model {
			return &providers[i], nil
		}
	}
	return nil, fmt.Errorf("unknown model %q", cfg.Model)
}

func buildTransport(cfg *config.Config) (*telegram.Transport, error) {
	switch cfg.Transport {
	case "telegram":
		if cfg.TelegramToken == "" {
			return nil, fmt.Errorf("TELEGRAM_BOT_TOKEN is required for telegram transport")
		}
		if cfg.TelegramOwnerID == 0 {
			return nil, fmt.Errorf("TELEGRAM_OWNER_ID is required for telegram transport")
		}
		return telegram.New(cfg.TelegramToken, cfg.TelegramOwnerID)
	default:
		return nil, fmt.Errorf("unknown transport: %s", cfg.Transport)
	}
}
