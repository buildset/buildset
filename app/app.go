package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/buildset/buildset/pkg/config"
	"github.com/buildset/buildset/pkg/serve"
)

// Expiry is enforced on every lookup; sweeping only keeps the table from growing forever.
const expiredSessionSweepInterval = time.Hour

// Application is the whole system assembled: configuration, storage, services, and one HTTP
// handler in front of them.
type Application struct {
	config   *Config
	logger   *slog.Logger
	stores   *Stores
	services *services
	routes   http.Handler
}

func New(ctx context.Context, cfg *Config, logger *slog.Logger) (*Application, error) {
	stores, err := OpenStores(ctx, cfg.Database)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	svc, err := newServices(ctx, cfg, stores, logger)
	if err != nil {
		_ = stores.Close()

		return nil, fmt.Errorf("build services: %w", err)
	}

	routes, err := Routes(cfg, svc, logger)
	if err != nil {
		_ = stores.Close()

		return nil, fmt.Errorf("build http handler: %w", err)
	}

	return &Application{
		config:   cfg,
		logger:   logger,
		stores:   stores,
		services: svc,
		routes:   routes,
	}, nil
}

// Handler wraps the routes with health probes and middleware.
func (a *Application) Handler() http.Handler {
	return serve.Handler(a.serveOptions())
}

func (a *Application) serveOptions() serve.Options {
	return serve.Options{
		Name:            "blog",
		Address:         a.config.Address(),
		ShutdownTimeout: a.config.ShutdownTimeout,
		Logger:          a.logger,
		Routes:          a.routes,
		Ready:           a.stores.Ping,
		CrossOrigin:     true,
		Background:      []func(context.Context){a.SweepExpiredSessions},
	}
}

func (a *Application) Close() error {
	return a.stores.Close()
}

// SweepExpiredSessions runs until the context is cancelled.
func (a *Application) SweepExpiredSessions(ctx context.Context) {
	ticker := time.NewTicker(expiredSessionSweepInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			deleted, err := a.services.auth.DeleteExpiredSessions(ctx)
			if err != nil {
				a.logger.WarnContext(ctx, "sweep expired sessions", slog.Any("error", err))

				continue
			}

			if deleted > 0 {
				a.logger.InfoContext(ctx, "swept expired sessions", slog.Int64("count", deleted))
			}
		}
	}
}

func Run(ctx context.Context) error {
	cfg, err := LoadConfig(ctx)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	logger := serve.NewLogger(config.Log{Level: cfg.LogLevel, JSON: cfg.LogJSON})
	slog.SetDefault(logger)

	application, err := New(ctx, cfg, logger)
	if err != nil {
		return err
	}
	defer func() {
		if err := application.Close(); err != nil {
			logger.ErrorContext(ctx, "close application", slog.Any("error", err))
		}
	}()

	return serve.Run(ctx, application.serveOptions())
}
