// Package authapp is the composition root of the identity service running on its own. It serves the
// sign-in pages and the small API the site calls on one port. Credential operations are reachable
// only from the pages, so nothing on the network can create a session or change a password.
package authapp

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/buildset/buildset/auth"
	"github.com/buildset/buildset/auth/hash"
	"github.com/buildset/buildset/auth/httpapi"
	authpostgres "github.com/buildset/buildset/auth/postgres"
	authsqlite "github.com/buildset/buildset/auth/sqlite"
	authui "github.com/buildset/buildset/auth/ui"
	authzclient "github.com/buildset/buildset/authz/client"
	"github.com/buildset/buildset/pkg/config"
	"github.com/buildset/buildset/pkg/httpx"
	"github.com/buildset/buildset/pkg/serve"
	"github.com/buildset/buildset/pkg/storage"
	"github.com/nasermirzaei89/env"
)

// schema is the Postgres schema this service owns. SQLite ignores it.
const schema = "auth"

var errInvalidConfig = errors.New("invalid configuration")

// Expiry is enforced on every lookup; sweeping only keeps the table from growing forever.
const expiredSessionSweepInterval = time.Hour

type Config struct {
	config.Server

	Log      config.Log
	Cookie   config.Cookie
	Database storage.Config

	BcryptCost       int
	SessionTTL       time.Duration
	RegistrationOpen bool
	// AdminRole is the application's vocabulary; authz only stores the string.
	AdminRole string

	AuthzURL    string
	HTTPTimeout time.Duration
}

func LoadConfig(ctx context.Context) (*Config, error) {
	log, err := config.LoadLog()
	if err != nil {
		return nil, err
	}

	authzURL, err := config.LoadURL("AUTHZ_URL")
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		Server:           config.LoadServer(),
		Log:              log,
		Cookie:           config.LoadCookie(),
		Database:         storage.Load(),
		BcryptCost:       env.GetInt("AUTH_BCRYPT_COST", 12),
		SessionTTL:       env.GetDuration("AUTH_SESSION_TTL", 14*24*time.Hour),
		RegistrationOpen: env.GetBool("AUTH_REGISTRATION_OPEN", false),
		AdminRole:        env.GetString("AUTH_ADMIN_ROLE", "admin"),
		AuthzURL:         authzURL,
		HTTPTimeout:      env.GetDuration("HTTP_TIMEOUT", 5*time.Second),
	}

	if err := cfg.Validate(ctx); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) Validate(ctx context.Context) error {
	if err := c.Server.Validate(ctx); err != nil {
		return err
	}

	if err := c.Database.Validate(); err != nil {
		return err
	}

	// bcrypt rejects costs outside [4, 31]; bounding here fails at boot rather than per login.
	if c.BcryptCost < 10 || c.BcryptCost > 31 {
		return fmt.Errorf(
			"%w: AUTH_BCRYPT_COST must be between 10 and 31, got %d",
			errInvalidConfig,
			c.BcryptCost,
		)
	}

	if c.SessionTTL <= 0 {
		return fmt.Errorf(
			"%w: AUTH_SESSION_TTL must be positive, got %s",
			errInvalidConfig,
			c.SessionTTL,
		)
	}

	if c.AdminRole == "" {
		return fmt.Errorf("%w: AUTH_ADMIN_ROLE must not be empty", errInvalidConfig)
	}

	return nil
}

type Service struct {
	db      *sql.DB
	service *auth.Service
	routes  http.Handler
	logger  *slog.Logger
}

func New(ctx context.Context, cfg *Config, logger *slog.Logger) (*Service, error) {
	db, err := storage.Open(ctx, cfg.Database, schema)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	service, routes, err := build(ctx, cfg, db, logger)
	if err != nil {
		_ = db.Close()

		return nil, err
	}

	return &Service{db: db, service: service, routes: routes, logger: logger}, nil
}

func build(
	ctx context.Context,
	cfg *Config,
	db *sql.DB,
	logger *slog.Logger,
) (*auth.Service, http.Handler, error) {
	bcryptAlgorithm, err := hash.NewBcrypt(cfg.BcryptCost)
	if err != nil {
		return nil, nil, fmt.Errorf("build bcrypt algorithm: %w", err)
	}

	// TODO: register argon2id here and prefer it once implemented. Existing bcrypt hashes keep
	// verifying, and each user is upgraded on their next login.
	passwords, err := hash.NewRegistry(bcryptAlgorithm)
	if err != nil {
		return nil, nil, fmt.Errorf("build password registry: %w", err)
	}

	repository, err := newRepository(ctx, cfg.Database.Driver, db)
	if err != nil {
		return nil, nil, fmt.Errorf("build auth repository: %w", err)
	}

	authzClient, err := authzclient.New(cfg.AuthzURL, httpx.ClientOptions{Timeout: cfg.HTTPTimeout})
	if err != nil {
		return nil, nil, fmt.Errorf("build authz client: %w", err)
	}

	service, err := auth.NewService(repository, passwords,
		firstUserHook(authzClient, cfg.AdminRole, logger), cfg.SessionTTL, logger)
	if err != nil {
		return nil, nil, fmt.Errorf("build auth service: %w", err)
	}

	pages, err := authui.NewHandler(
		service,
		registrationPolicy(cfg.RegistrationOpen, authzClient),
		authui.Config{
			SessionCookieName: cfg.Cookie.Name,
			SecureCookies:     cfg.Cookie.Secure,
		},
		logger,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("build auth handler: %w", err)
	}

	api, err := httpapi.NewHandler(service, logger)
	if err != nil {
		return nil, nil, fmt.Errorf("build auth api: %w", err)
	}

	mux := http.NewServeMux()
	// The pages keep unprefixed paths. Routing them at the gateway gives the browser one origin,
	// which is what keeps the session cookie and the same-origin checks working.
	pages.Register(mux)
	api.Register(mux)

	return service, mux, nil
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

// SweepExpiredSessions runs until the context is cancelled.
func (s *Service) SweepExpiredSessions(ctx context.Context) {
	ticker := time.NewTicker(expiredSessionSweepInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			deleted, err := s.service.DeleteExpiredSessions(ctx)
			if err != nil {
				s.logger.WarnContext(ctx, "sweep expired sessions", slog.Any("error", err))

				continue
			}

			if deleted > 0 {
				s.logger.InfoContext(ctx, "swept expired sessions", slog.Int64("count", deleted))
			}
		}
	}
}

func Run(ctx context.Context) error {
	cfg, err := LoadConfig(ctx)
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
		Name:            "auth",
		Address:         cfg.Address(),
		ShutdownTimeout: cfg.ShutdownTimeout,
		Logger:          logger,
		Routes:          service.Routes(),
		Ready:           service.Ping,
		// A browser posts sign-in forms here through the gateway, so cross-origin protection
		// applies, and an inbound request identifier must not be trusted.
		CrossOrigin:    true,
		TrustRequestID: false,
		Background:     []func(context.Context){service.SweepExpiredSessions},
	})
}

func newRepository(ctx context.Context, driver string, db *sql.DB) (auth.Repository, error) {
	if driver == storage.DriverPostgres {
		return authpostgres.NewRepository(ctx, db)
	}

	return authsqlite.NewRepository(ctx, db)
}
