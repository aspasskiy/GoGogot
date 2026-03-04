package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

type Config struct {
	Transport       string // "telegram" (default), "tui", "http", "slack", ...
	TelegramToken   string
	TelegramOwnerID int64
	BraveAPIKey     string
	DataDir         string
	LogLevel        string
	Model           string
	MaxTokens       int
}

func Load() (*Config, error) {
	transport := os.Getenv("GOGOGOT_TRANSPORT")
	if transport == "" {
		transport = "telegram"
	}

	cfg := &Config{
		Transport:     transport,
		TelegramToken: os.Getenv("TELEGRAM_BOT_TOKEN"),
		BraveAPIKey:   os.Getenv("BRAVE_API_KEY"),
		DataDir:       os.Getenv("GOGOGOT_DATA_DIR"),
		LogLevel:      os.Getenv("LOG_LEVEL"),
		Model:         os.Getenv("GOGOGOT_MODEL"),
		MaxTokens:     4096,
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
		cfg.TelegramOwnerID = id
	}

	if s := os.Getenv("GOGOGOT_MAX_TOKENS"); s != "" {
		v, err := strconv.Atoi(s)
		if err == nil && v > 0 {
			cfg.MaxTokens = v
		}
	}

	return cfg, nil
}

