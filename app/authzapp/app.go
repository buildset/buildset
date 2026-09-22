// Package authzapp is the composition root of the authorization service running on its own.
//
// It is a package rather than a main so a test can build the routes without a listener.
package authzapp

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/buildset/buildset/authz"
	"github.com/buildset/buildset/authz/httpapi"
	authzpostgres "github.com/buildset/buildset/authz/postgres"
	authzsqlite "github.com/buildset/buildset/authz/sqlite"
	"github.com/buildset/buildset/pkg/config"
	"github.com/buildset/buildset/pkg/serve"
	"github.com/buildset/buildset/pkg/storage"
)

// schema is the Postgres schema this service owns. SQLite ignores it.
const schema = "authz"

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

// Service is this binary assembled: storage, the service, and the routes in front of it.
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
		db.Close()

		return nil, fmt.Errorf("build authz repository: %w", err)
	}

	handler, err := httpapi.NewHandler(authz.NewService(repository), logger)
	if err != nil {
		db.Close()

		return nil, fmt.Errorf("build authz handler: %w", err)
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
	defer service.Close()

	return serve.Run(ctx, serve.Options{
		Name:            "authz",
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

func newRepository(ctx context.Context, driver string, db *sql.DB) (authz.Repository, error) {
	if driver == storage.DriverPostgres {
		return authzpostgres.NewRepository(ctx, db)
	}

	return authzsqlite.NewRepository(ctx, db)
}
