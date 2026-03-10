package main

import (
	"context"
	"fmt"
	"gogogot/internal/channel/telegram"
	"gogogot/internal/core"
	"gogogot/internal/infra/config"
	"gogogot/internal/infra/logger"
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

	eng, err := core.New(cfg, ch)
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

func notifyOwnerAndBlock(ch *telegram.Channel, providerErr error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, reply := ch.OwnerSession()
	msg := fmt.Sprintf("⚠️ Failed to start:\n\n%v\n\nFix environment variables and restart the container.", providerErr)
	_ = reply.SendText(ctx, msg)

	fmt.Fprintf(os.Stderr, "error: %v\nBlocking to prevent restart loop. Fix env vars and restart manually.\n", providerErr)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
}

func buildChannel(cfg *config.Config) (*telegram.Channel, error) {
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
