package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// TelegramConfig is read only to validate the setup early: the telegram
// channel skill itself takes these from the environment.
type TelegramConfig struct {
	Token   string
	OwnerID int64
}

type LLMConfig struct {
	Model     string
	Provider  string // "anthropic", "openai", or "openrouter"
	MaxTokens int
	Timeout   time.Duration // per-LLM-call timeout (GOGOGOT_LLM_TIMEOUT, default 3m)
}

type SchedulerConfig struct {
	TaskTimeout time.Duration
	// MaxConcurrent caps tasks waiting for the agent at once. The engine runs
	// one agent at a time, so values above 1 only make tasks queue inside the
	// engine, where the wait counts against their timeout.
	MaxConcurrent int
}

type Config struct {
	// Transport says what the owner talks through, comma-separated:
	// "terminal", "telegram" (requires the telegram channel skill to start), or
	// "auto" (the default) — the terminal when running interactively. Channel
	// skills, telegram included, start on their own according to their
	// channel.json and the env vars it requires.
	Transport   string
	DataDir     string
	LogLevel    string
	BraveAPIKey string

	Telegram  TelegramConfig
	LLM       LLMConfig
	Scheduler SchedulerConfig
}

func envDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func Load() (*Config, error) {
	cfg := &Config{
		Transport:   envDefault("GOGOGOT_TRANSPORT", "auto"),
		BraveAPIKey: os.Getenv("BRAVE_API_KEY"),
		DataDir:     os.Getenv("GOGOGOT_DATA_DIR"),
		LogLevel:    envDefault("LOG_LEVEL", "debug"),

		Telegram: TelegramConfig{
			Token: os.Getenv("TELEGRAM_BOT_TOKEN"),
		},
		LLM: LLMConfig{
			Model:     os.Getenv("GOGOGOT_MODEL"),
			Provider:  os.Getenv("GOGOGOT_PROVIDER"),
			MaxTokens: 4096,
			Timeout:   3 * time.Minute,
		},
		Scheduler: SchedulerConfig{
			TaskTimeout:   5 * time.Minute,
			MaxConcurrent: 1,
		},
	}

	if cfg.DataDir == "" {
		home, _ := os.UserHomeDir()
		cfg.DataDir = filepath.Join(home, ".gogogot")
	}

	if s := os.Getenv("TELEGRAM_OWNER_ID"); s != "" {
		id, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid TELEGRAM_OWNER_ID: %w", err)
		}
		cfg.Telegram.OwnerID = id
	}

	if err := positiveInt("GOGOGOT_MAX_TOKENS", &cfg.LLM.MaxTokens); err != nil {
		return nil, err
	}
	if err := positiveDuration("GOGOGOT_LLM_TIMEOUT", &cfg.LLM.Timeout); err != nil {
		return nil, err
	}
	if err := positiveDuration("GOGOGOT_SCHEDULER_TASK_TIMEOUT", &cfg.Scheduler.TaskTimeout); err != nil {
		return nil, err
	}
	if err := positiveInt("GOGOGOT_SCHEDULER_MAX_CONCURRENT", &cfg.Scheduler.MaxConcurrent); err != nil {
		return nil, err
	}

	return cfg, nil
}

// positiveInt overrides *dst with the env var when set; a set value that is
// not a positive integer is an error.
func positiveInt(key string, dst *int) error {
	s := os.Getenv(key)
	if s == "" {
		return nil
	}
	v, err := strconv.Atoi(s)
	if err != nil || v <= 0 {
		return fmt.Errorf("invalid %s %q: want a positive integer", key, s)
	}
	*dst = v
	return nil
}

// positiveDuration overrides *dst with the env var when set; a set value that
// is not a positive duration (e.g. "90s", "5m") is an error.
func positiveDuration(key string, dst *time.Duration) error {
	s := os.Getenv(key)
	if s == "" {
		return nil
	}
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		return fmt.Errorf("invalid %s %q: want a positive duration like 90s or 5m", key, s)
	}
	*dst = d
	return nil
}
