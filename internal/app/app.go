package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/FenkoHQ/vulpes-core/internal/config"
	"github.com/FenkoHQ/vulpes-core/internal/httpapi"
	"github.com/FenkoHQ/vulpes-core/internal/models"
	"github.com/FenkoHQ/vulpes-core/internal/pipeline"
	"github.com/FenkoHQ/vulpes-core/internal/plugins"
	"github.com/FenkoHQ/vulpes-core/internal/secrets"
)

type App struct {
	Config   *config.Config
	Plugins  *plugins.Manager
	Pipeline *pipeline.Pipeline
	Server   *http.Server
	logger   *slog.Logger
}

func New(ctx context.Context, cfg *config.Config, logger *slog.Logger) (*App, error) {
	if logger == nil {
		logger = slog.Default()
	}
	sp := secrets.EnvProvider{Enabled: cfg.Secrets.Env.Enabled}
	pm := plugins.NewManager(cfg.Plugins, sp, logger)
	if len(cfg.Plugins) > 0 {
		if err := pm.Start(ctx); err != nil {
			return nil, err
		}
	} else if !cfg.AllowZeroPlugins {
		logger.Warn("starting with zero plugins; gateway will not be ready for inference")
	}
	mr := models.NewRegistry(cfg.Models)
	pl, err := pipeline.Compile(*cfg, pm.Registry(), mr)
	if err != nil {
		return nil, fmt.Errorf("compile pipeline: %w", err)
	}
	h := httpapi.New(pl, logger).Handler()
	srv := httpapi.NewHTTPServer(cfg.Server.Listen, h, cfg.Server.RequestTimeout.Duration())
	return &App{Config: cfg, Plugins: pm, Pipeline: pl, Server: srv, logger: logger}, nil
}

func (a *App) Run(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() { a.logger.Info("gateway listening", "addr", a.Server.Addr); errCh <- a.Server.ListenAndServe() }()
	select {
	case <-ctx.Done():
		sctx, cancel := context.WithTimeout(context.Background(), a.Config.Server.RequestTimeout.Duration())
		defer cancel()
		_ = a.Server.Shutdown(sctx)
		_ = a.Plugins.Stop(sctx)
		return ctx.Err()
	case err := <-errCh:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	}
}
