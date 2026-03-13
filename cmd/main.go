package main

import (
	"context"
	"fmt"
	"gogogot/internal/channel"
	"gogogot/internal/channel/telegram"
	"gogogot/internal/core"
	"gogogot/internal/core/agent"
	"gogogot/internal/core/agent/hook"
	"gogogot/internal/core/episode"
	"gogogot/internal/core/prompt"
	"gogogot/internal/infra/config"
	"gogogot/internal/infra/logger"
	"gogogot/internal/infra/scheduler"
	"gogogot/internal/llm"
	"gogogot/internal/tools"
	"gogogot/internal/tools/store"
	"gogogot/internal/tools/store/local"
	"gogogot/internal/tools/system"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog/log"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	logger.Init(cfg.LogLevel)

	ch, err := buildChannel(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	eng, err := buildEngine(cfg, ch)
	if err != nil {
		notifyOwnerAndBlock(ch, err)
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	fmt.Printf("Sofie is running [%s]. Press Ctrl+C to stop.\n", ch.Name())
	if err := eng.Run(ctx); err != nil && ctx.Err() == nil {
		log.Error().Err(err).Msg("engine run error")
	}
	fmt.Println("Shutting down.")
}

func notifyOwnerAndBlock(ch channel.Channel, providerErr error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	reply := ch.OwnerReplier()
	msg := fmt.Sprintf("⚠️ Failed to start:\n\n%v\n\nFix environment variables and restart the container.", providerErr)
	_ = reply.SendText(ctx, msg)

	fmt.Fprintf(os.Stderr, "error: %v\nBlocking to prevent restart loop. Fix env vars and restart manually.\n", providerErr)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
}

func buildEngine(cfg *config.Config, ch channel.Channel) (*core.Engine, error) {
	st, err := local.New(cfg.DataDir)
	if err != nil {
		return nil, fmt.Errorf("init store: %w", err)
	}

	provider, err := resolveProvider(cfg)
	if err != nil {
		return nil, err
	}

	sched := scheduler.New(cfg.DataDir, nil, st.LoadTimezone(), scheduler.Options{
		TaskTimeout:   cfg.Scheduler.TaskTimeout,
		MaxConcurrent: cfg.Scheduler.MaxConcurrent,
	})

	extra := append(tools.ChannelTools(),
		system.ScheduleTools(sched)...,
	)
	extra = append(extra, tools.IdentityTools(st, sched.SetLocation)...)

	client := llm.NewClient(*provider, nil)
	epMgr := episode.NewManager(st, client)

	reg := tools.NewRegistry(st, cfg.BraveAPIKey, epMgr.SearchRelevant, extra...)
	client.SetTools(reg.Definitions())

	transportName := ch.Name()
	modelLabel := provider.Label
	instructions := func() string {
		skills, _ := st.LoadSkills()
		return prompt.SystemPrompt(prompt.PromptContext{
			TransportName: transportName,
			ModelLabel:    modelLabel,
			Soul:          st.ReadSoul(),
			User:          st.ReadUser(),
			SkillsBlock:   store.FormatSkillsForPrompt(skills),
			Timezone:      st.LoadTimezone(),
		})
	}

	ag := agent.New(client, instructions, reg)
	ag.AddBeforeHook(hook.NewCompaction().BeforeHook())

	return core.New(core.Params{
		Channel:   ch,
		Store:     st,
		Agent:     ag,
		Episodes:  epMgr,
		Scheduler: sched,
		Registry:  reg,
	}), nil
}

func resolveProvider(cfg *config.Config) (*llm.Provider, error) {
	if cfg.LLM.Provider == "" {
		return nil, fmt.Errorf("GOGOGOT_PROVIDER is required — set to 'anthropic', 'openai', or 'openrouter'")
	}
	if cfg.LLM.Model == "" {
		return nil, fmt.Errorf("GOGOGOT_MODEL is required — use an exact model ID (e.g. claude-sonnet-4-6, gpt-4o) or an OpenRouter slug (vendor/model)")
	}
	return llm.ResolveProvider(cfg.LLM.Model, cfg.LLM.Provider)
}

func buildChannel(cfg *config.Config) (channel.Channel, error) {
	switch cfg.Transport {
	case "telegram":
		if cfg.Telegram.Token == "" {
			return nil, fmt.Errorf("TELEGRAM_BOT_TOKEN is required for telegram transport")
		}
		if cfg.Telegram.OwnerID == 0 {
			return nil, fmt.Errorf("TELEGRAM_OWNER_ID is required for telegram transport")
		}
		return telegram.New(cfg.Telegram.Token, cfg.Telegram.OwnerID)
	default:
		return nil, fmt.Errorf("unknown transport: %s", cfg.Transport)
	}
}
