package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/aspasskiy/gogogot/internal/channel"
	"github.com/aspasskiy/gogogot/internal/channel/bridge"
	"github.com/aspasskiy/gogogot/internal/channel/terminal"
	"github.com/aspasskiy/gogogot/internal/config"
	"github.com/aspasskiy/gogogot/internal/core"
	"github.com/aspasskiy/gogogot/internal/core/agent"
	"github.com/aspasskiy/gogogot/internal/core/agent/hook"
	"github.com/aspasskiy/gogogot/internal/core/chat"
	"github.com/aspasskiy/gogogot/internal/core/prompt"
	bundledskills "github.com/aspasskiy/gogogot/internal/data/skills"
	"github.com/aspasskiy/gogogot/internal/domain"
	"github.com/aspasskiy/gogogot/internal/llm"
	"github.com/aspasskiy/gogogot/internal/logger"
	"github.com/aspasskiy/gogogot/internal/scheduler"
	"github.com/aspasskiy/gogogot/internal/store/local"
	"github.com/aspasskiy/gogogot/internal/tools"
	channeltools "github.com/aspasskiy/gogogot/internal/tools/channels"
	"github.com/aspasskiy/gogogot/internal/tools/comm"
	"github.com/aspasskiy/gogogot/internal/tools/identity"
	"github.com/aspasskiy/gogogot/internal/tools/system"

	"github.com/rs/zerolog/log"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	sel, err := selectChannels(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	// The terminal channel owns the screen, so logs go to a file then.
	var logPath string
	if sel.terminal {
		logPath = filepath.Join(cfg.DataDir, "gogogot.log")
	}
	if err := logger.Init(cfg.LogLevel, logPath); err != nil {
		fmt.Fprintf(os.Stderr, "error: open log file: %v\n", err)
		os.Exit(1)
	}

	chans, err := buildChannels(cfg, sel, logPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	// The terminal handles Ctrl+C itself (stop the task, twice to quit).
	sigs := []os.Signal{syscall.SIGTERM}
	if !sel.terminal {
		sigs = append(sigs, os.Interrupt)
	}
	ctx, stop := signal.NotifyContext(context.Background(), sigs...)
	defer stop()

	if chans.telegram != "" {
		// Fail fast like before the channel became a skill: a wrong token
		// must stop the container (visible in docker logs), not leave a
		// silent restart loop.
		chans.skills.Start(ctx)
		if s, _ := chans.skills.WaitSettled(chans.telegram, 15*time.Second); s.State == bridge.StateCrashed {
			logs, _ := chans.skills.Logs(chans.telegram)
			if len(logs) > 10 {
				logs = logs[len(logs)-10:]
			}
			reason, _, _ := strings.Cut(s.Detail, "; restarting")
			fmt.Fprintf(os.Stderr, "error: telegram channel failed to start (%s):\n%s\n", reason, strings.Join(logs, "\n"))
			os.Exit(1)
		}
	}

	eng, err := buildEngine(cfg, chans)
	if err != nil {
		notifyOwnerAndBlock(ctx, chans, sel.terminal, err)
		return
	}

	log.Info().Str("channels", chans.all.Name()).Msg("Sofie is running")
	if !sel.terminal {
		fmt.Printf("Sofie is running [%s]. Press Ctrl+C to stop.\n", chans.all.Name())
	}
	if err := eng.Run(ctx); err != nil && ctx.Err() == nil {
		log.Error().Err(err).Msg("engine run error")
	}
	if !sel.terminal {
		fmt.Println("Shutting down.")
	}
}

// channelSelection is what must run at startup.
type channelSelection struct {
	terminal bool
	// telegram: the telegram channel skill must start — its token is set, or
	// GOGOGOT_TRANSPORT asks for it. Startup fails loudly if it cannot.
	telegram bool
}

// selectChannels resolves GOGOGOT_TRANSPORT. "auto" (the default) runs the
// terminal when interactive and requires Telegram when its token is set; an
// explicit list must be satisfiable. Other channel skills are not selected
// here: they start according to their channel.json.
func selectChannels(cfg *config.Config) (channelSelection, error) {
	var sel channelSelection
	for _, name := range strings.Split(cfg.Transport, ",") {
		switch strings.TrimSpace(name) {
		case "auto":
			sel.telegram = sel.telegram || cfg.Telegram.Token != ""
			sel.terminal = sel.terminal || terminal.Interactive()
		case "telegram":
			if cfg.Telegram.Token == "" {
				return sel, fmt.Errorf("TELEGRAM_BOT_TOKEN is required for telegram transport")
			}
			sel.telegram = true
		case "terminal":
			if !terminal.Interactive() {
				return sel, fmt.Errorf("terminal transport needs an interactive terminal (docker run -it ...)")
			}
			sel.terminal = true
		case "":
		default:
			return sel, fmt.Errorf("unknown transport: %s (want auto, telegram or terminal)", name)
		}
	}

	if sel.telegram && cfg.Telegram.OwnerID == 0 {
		return sel, fmt.Errorf("TELEGRAM_OWNER_ID is required for telegram transport")
	}
	return sel, nil
}

// channels is everything the owner can talk through: the built-in terminal
// plus the channel skills, combined into one for the engine.
type channels struct {
	all      channel.Channel
	skills   *bridge.Manager
	builtin  []string
	telegram string // the telegram channel skill's name, when it must run
}

func buildChannels(cfg *config.Config, sel channelSelection, logPath string) (*channels, error) {
	skillsDir := filepath.Join(cfg.DataDir, "skills")
	installed, err := bundledskills.Install(skillsDir)
	if err != nil {
		log.Error().Err(err).Msg("install bundled skills")
	}

	var list []channel.Channel
	var builtin []string
	if sel.terminal {
		t := terminal.New()
		t.LogPath = logPath
		list = append(list, t)
		builtin = append(builtin, t.Name())
	}

	skills := bridge.NewManager(skillsDir, builtin...)
	skills.Rollback = func(name string) ([]string, error) { return bundledskills.Restore(skillsDir, name) }

	var telegramName string
	if sel.telegram {
		name := installed["telegram"]
		telegramName = name
		if name == "" {
			return nil, fmt.Errorf("the telegram channel skill could not be installed into %s (see the log)", skillsDir)
		}
		if s, ok := skills.Status(name); !ok || !s.Runnable() {
			return nil, fmt.Errorf("telegram channel (skill %q) cannot start: %s", name, s.Detail)
		}
	}
	if len(list) == 0 && !skills.Runnable() {
		return nil, fmt.Errorf("no channel to talk through: set TELEGRAM_BOT_TOKEN and TELEGRAM_OWNER_ID, " +
			"or run in an interactive terminal (docker run -it ...)")
	}
	list = append(list, skills)

	return &channels{all: channel.NewMulti(list...), skills: skills, builtin: builtin, telegram: telegramName}, nil
}

func notifyOwnerAndBlock(ctx context.Context, chans *channels, interactive bool, providerErr error) {
	// Channel skills only deliver once their bridge is up.
	chans.skills.Start(ctx)
	chans.skills.WaitReady(15 * time.Second)

	sendCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	reply := chans.all.OwnerReplier()
	msg := fmt.Sprintf("⚠️ Failed to start:\n\n%v\n\nFix environment variables and restart the container.", providerErr)
	_ = reply.SendText(sendCtx, msg)

	if interactive {
		// Nothing restarts an interactive run in a loop; just exit.
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "error: %v\nBlocking to prevent restart loop. Fix env vars and restart manually.\n", providerErr)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
}

func buildEngine(cfg *config.Config, chans *channels) (*core.Engine, error) {
	ch := chans.all
	st, err := local.New(cfg.DataDir)
	if err != nil {
		return nil, fmt.Errorf("init store: %w", err)
	}

	provider, err := resolveProvider(cfg)
	if err != nil {
		return nil, err
	}
	provider.Timeout = cfg.LLM.Timeout
	provider.MaxTokens = cfg.LLM.MaxTokens

	sched := scheduler.New(cfg.DataDir, nil, st.LoadTimezone(), scheduler.Options{
		TaskTimeout:   cfg.Scheduler.TaskTimeout,
		MaxConcurrent: cfg.Scheduler.MaxConcurrent,
	})

	extra := append(comm.ChannelTools(),
		system.ScheduleTools(sched)...,
	)
	extra = append(extra, identity.IdentityTools(st, sched.SetLocation)...)
	extra = append(extra, channeltools.ChannelTools(chans.skills, chans.builtin)...)

	client := llm.NewClient(*provider, nil)
	chatMgr := chat.NewManager(st, client)

	reg := tools.NewRegistry(st, cfg.BraveAPIKey, chatMgr.SearchRelevant, extra...)
	client.SetTools(reg.Definitions())

	modelLabel := provider.Label
	instructions := func() string {
		skills, _ := st.LoadSkills()
		return prompt.SystemPrompt(prompt.PromptContext{
			TransportName: ch.Name(), // channel skills can come and go at runtime
			ModelLabel:    modelLabel,
			Soul:          st.ReadSoul(),
			User:          st.ReadUser(),
			SkillsBlock:   domain.FormatSkillsForPrompt(skills),
			Timezone:      st.LoadTimezone(),
		})
	}

	ag := agent.New(client, instructions, reg)
	ag.AddBeforeHook(hook.NewCompaction().BeforeHook())

	return core.New(core.Params{
		Channel:   ch,
		Store:     st,
		Agent:     ag,
		Chats:     chatMgr,
		Scheduler: sched,
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
