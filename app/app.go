package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// expiredSessionSweepInterval is how often sessions past their expiry are removed. Expiry is
// already enforced on every lookup; this only keeps the table from growing forever.
const expiredSessionSweepInterval = time.Hour

// Application is the whole system assembled: configuration, storage, services, and one HTTP
// handler in front of them. Run serves it; a test can hold it and serve it however it likes.
type Application struct {
	config   *Config
	logger   *slog.Logger
	stores   *Stores
	services *services
	handler  http.Handler
}

func New(ctx context.Context, cfg *Config, logger *slog.Logger) (*Application, error) {
	stores, err := OpenStores(ctx, cfg.Database)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	svc, err := newServices(ctx, cfg, stores, logger)
	if err != nil {
		stores.Close()

		return nil, fmt.Errorf("build services: %w", err)
	}

	handler, err := NewHandler(cfg, stores, svc, logger)
	if err != nil {
		stores.Close()

		return nil, fmt.Errorf("build http handler: %w", err)
	}

	return &Application{config: cfg, logger: logger, stores: stores, services: svc, handler: handler}, nil
}

func (a *Application) Handler() http.Handler {
	return a.handler
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
	cfg, err := LoadConfig()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	logger := NewLogger(cfg)
	slog.SetDefault(logger)

	application, err := New(ctx, cfg, logger)
	if err != nil {
		return err
	}
	defer application.Close()

	go application.SweepExpiredSessions(ctx)

	server := &http.Server{
		Addr:              cfg.Address(),
		Handler:           application.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       2 * time.Minute,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelWarn),
	}

	errs := make(chan error, 1)

	go func() {
		logger.Info("http server listening", slog.String("address", cfg.Address()))

		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errs <- fmt.Errorf("listen and serve: %w", err)

			return
		}

		errs <- nil
	}()

	select {
	case err := <-errs:
		return err
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	}

	// The shutdown deadline must survive the cancelled context that triggered it.
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cfg.ShutdownTimeout)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown http server: %w", err)
	}

	return <-errs
}
