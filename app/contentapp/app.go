// Package contentapp is the composition root of the content service running on its own. It is a
// package rather than a main so a test can build the routes without a listener.
package contentapp

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/buildset/buildset/content"
	"github.com/buildset/buildset/content/httpapi"
	contentpostgres "github.com/buildset/buildset/content/postgres"
	contentsqlite "github.com/buildset/buildset/content/sqlite"
	"github.com/buildset/buildset/pkg/config"
	"github.com/buildset/buildset/pkg/serve"
	"github.com/buildset/buildset/pkg/storage"
)

// schema is the Postgres schema this service owns. SQLite ignores it.
const schema = "content"

type Config struct {
	config.Server

	Log      config.Log
	Database storage.Config
}

func LoadConfig() (*Config, error) {
	log, err := config.LoadLog()
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		Server:   config.LoadServer(),
		Log:      log,
		Database: storage.Load(),
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) Validate() error {
	if err := c.Server.Validate(); err != nil {
		return err
	}

	return c.Database.Validate()
}

type Service struct {
	db     *sql.DB
	routes http.Handler
}

func New(ctx context.Context, cfg *Config, logger *slog.Logger) (*Service, error) {
	db, err := storage.Open(ctx, cfg.Database, schema)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	repository, err := newRepository(ctx, cfg.Database.Driver, db)
	if err != nil {
		_ = db.Close()

		return nil, fmt.Errorf("build content repository: %w", err)
	}

	handler, err := httpapi.NewHandler(content.NewService(repository), logger)
	if err != nil {
		_ = db.Close()

		return nil, fmt.Errorf("build content handler: %w", err)
	}

	mux := http.NewServeMux()
	handler.Register(mux)

	return &Service{db: db, routes: mux}, nil
}

func (s *Service) Routes() http.Handler { return s.routes }

func (s *Service) Ping(ctx context.Context) error {
	if err := s.db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}

	return nil
}

func (s *Service) Close() error {
	if err := s.db.Close(); err != nil {
		return fmt.Errorf("close database: %w", err)
	}

	return nil
}

func Run(ctx context.Context) error {
	cfg, err := LoadConfig()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	logger := serve.NewLogger(cfg.Log)
	slog.SetDefault(logger)

	service, err := New(ctx, cfg, logger)
	if err != nil {
		return err
	}
	defer func() {
		if err := service.Close(); err != nil {
			logger.ErrorContext(ctx, "close service", slog.Any("error", err))
		}
	}()

	return serve.Run(ctx, serve.Options{
		Name:            "content",
		Address:         cfg.Address(),
		ShutdownTimeout: cfg.ShutdownTimeout,
		Logger:          logger,
		Routes:          service.Routes(),
		Ready:           service.Ping,
		// No browser reaches this service, so there is no cross-origin form to protect, and the
		// callers are siblings whose request identifier is worth keeping.
		CrossOrigin:    false,
		TrustRequestID: true,
	})
}

func newRepository(ctx context.Context, driver string, db *sql.DB) (content.Repository, error) {
	if driver == storage.DriverPostgres {
		return contentpostgres.NewRepository(ctx, db)
	}

	return contentsqlite.NewRepository(ctx, db)
}
