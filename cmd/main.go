package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"gogogot/core/agent"
	"gogogot/core/agent/prompt"
	"gogogot/store"
	"gogogot/infra/config"
	"gogogot/llm"
	"gogogot/infra/logger"
	"gogogot/infra/scheduler"
	"gogogot/core/bridge"
	"gogogot/transport/telegram"
	"gogogot/tools"
	"gogogot/tools/system"

	"github.com/rs/zerolog/log"
)

func main() {
	modelFlag := flag.String("model", "", "model alias or OpenRouter slug (required if GOGOGOT_MODEL not set)")
	providerFlag := flag.String("provider", "", "LLM provider: anthropic or openrouter (required if GOGOGOT_PROVIDER not set)")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	if *modelFlag != "" {
		cfg.Model = *modelFlag
	}
	if *providerFlag != "" {
		cfg.Provider = *providerFlag
	}

	logger.Init(cfg.LogLevel)

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

	sched := scheduler.New(cfg.DataDir, nil, store.LoadTimezone())
	system.OnTimezoneChange = sched.SetLocation

	allTools := coreTools(cfg.BraveAPIKey, sched)
	allTools = append(allTools, bridge.TransportTools()...)
	reg := tools.NewRegistry(allTools)

	client := llm.NewClient(*provider, reg.Definitions())
	agentCfg := agent.AgentConfig{
		PromptCtx: prompt.PromptContext{
			TransportName: t.Name(),
			ModelLabel:    provider.Label,
		},
		MaxTokens:  4096,
		Compaction: agent.DefaultCompaction(),
	}

	b := bridge.New(t, client, agentCfg, reg)

	sched.SetExecutor(func(ctx context.Context, taskID, command, skill string) (string, error) {
		return b.RunScheduledTask(ctx, ownerChannelID, taskID, command, skill)
	})

	if err := sched.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "error starting scheduler: %v\n", err)
		os.Exit(1)
	}
	defer sched.Stop()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	fmt.Printf("Sofie is running [%s, %s]. Press Ctrl+C to stop.\n", t.Name(), provider.Label)
	if err := b.Run(ctx); err != nil && ctx.Err() == nil {
		log.Error().Err(err).Msg("bridge run error")
	}
	fmt.Println("Shutting down.")
}

func selectProvider(cfg *config.Config) (*llm.Provider, error) {
	if cfg.Provider == "" {
		return nil, fmt.Errorf("GOGOGOT_PROVIDER is required — set to 'anthropic' or 'openrouter'")
	}
	if cfg.Model == "" {
		return nil, fmt.Errorf("GOGOGOT_MODEL is required — use an alias (claude, deepseek, gemini, ...) or an OpenRouter slug")
	}
	return llm.ResolveProvider(cfg.Model, cfg.Provider)
}

type storeAdapter struct{}

func (storeAdapter) ListChats() ([]telegram.ChatInfo, error) {
	chats, err := store.ListChats()
	if err != nil {
		return nil, err
	}
	out := make([]telegram.ChatInfo, len(chats))
	for i, c := range chats {
		out[i] = telegram.ChatInfo{ID: c.ID, Title: c.Title, UpdatedAt: c.UpdatedAt}
	}
	return out, nil
}

func (storeAdapter) GetExternalMapping(channelID string) (string, error) {
	return store.GetExternalMapping(channelID)
}

func (storeAdapter) ListMemory() ([]telegram.MemoryFileInfo, error) {
	files, err := store.ListMemory()
	if err != nil {
		return nil, err
	}
	out := make([]telegram.MemoryFileInfo, len(files))
	for i, f := range files {
		out[i] = telegram.MemoryFileInfo{Name: f.Name, Size: f.Size}
	}
	return out, nil
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
		sa := storeAdapter{}
		return telegram.New(cfg.TelegramToken, cfg.TelegramOwnerID, sa, sa)
	default:
		return nil, fmt.Errorf("unknown transport: %s", cfg.Transport)
	}
}
