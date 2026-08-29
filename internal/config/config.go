package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

type TelegramConfig struct {
	Token   string
	OwnerID int64
}

type LLMConfig struct {
	Model     string
	Provider  string // "anthropic", "openai", or "openrouter"
	MaxTokens int
	Timeout   time.Duration // per-LLM-call timeout (GOGOGOT_LLM_TIMEOUT; 0 -> 3m default)
}

type SchedulerConfig struct {
	TaskTimeout   time.Duration
	MaxConcurrent int
}

type Config struct {
	Transport   string // "telegram" (default), "tui", "http", "slack", ...
	DataDir     string
	LogLevel    string
	BraveAPIKey  string
	TavilyAPIKey string

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
	transport := os.Getenv("GOGOGOT_TRANSPORT")
	if transport == "" {
		transport = "telegram"
	}

	cfg := &Config{
		Transport:   transport,
		BraveAPIKey:  os.Getenv("BRAVE_API_KEY"),
		TavilyAPIKey: os.Getenv("TAVILY_API_KEY"),
		DataDir:     os.Getenv("GOGOGOT_DATA_DIR"),
		LogLevel:    envDefault("LOG_LEVEL", "debug"),

		Telegram: TelegramConfig{
			Token: os.Getenv("TELEGRAM_BOT_TOKEN"),
		},
		LLM: LLMConfig{
			Model:     os.Getenv("GOGOGOT_MODEL"),
			Provider:  os.Getenv("GOGOGOT_PROVIDER"),
			MaxTokens: 4096,
		},
		Scheduler: SchedulerConfig{
			TaskTimeout:   5 * time.Minute,
			MaxConcurrent: 2,
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

	if s := os.Getenv("GOGOGOT_MAX_TOKENS"); s != "" {
		v, err := strconv.Atoi(s)
		if err == nil && v > 0 {
			cfg.LLM.MaxTokens = v
		}
	}

	if s := os.Getenv("GOGOGOT_LLM_TIMEOUT"); s != "" {
		if d, err := time.ParseDuration(s); err == nil && d > 0 {
			cfg.LLM.Timeout = d
		}
	}
	if cfg.LLM.Timeout <= 0 {
		cfg.LLM.Timeout = 3 * time.Minute
	}

	if s := os.Getenv("GOGOGOT_SCHEDULER_TASK_TIMEOUT"); s != "" {
		d, err := time.ParseDuration(s)
		if err == nil && d > 0 {
			cfg.Scheduler.TaskTimeout = d
		}
	}

	if s := os.Getenv("GOGOGOT_SCHEDULER_MAX_CONCURRENT"); s != "" {
		v, err := strconv.Atoi(s)
		if err == nil && v > 0 {
			cfg.Scheduler.MaxConcurrent = v
		}
	}

	return cfg, nil
}
