package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/FenkoHQ/vulpes-core/internal/app"
	"github.com/FenkoHQ/vulpes-core/internal/config"
)

func main() {
	configPath := flag.String("config", "gateway.yaml", "path to gateway YAML config")
	flag.Parse()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg, err := config.Load(*configPath)
	if err != nil {
		logger.Error("load config", "err", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	a, err := app.New(ctx, cfg, logger)
	if err != nil {
		logger.Error("initialize gateway", "err", err)
		os.Exit(1)
	}
	if err := a.Run(ctx); err != nil && ctx.Err() == nil {
		logger.Error("gateway failed", "err", err)
		os.Exit(1)
	}
}
