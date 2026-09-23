package app

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/buildset/buildset/auth"
	"github.com/buildset/buildset/auth/hash"
	authpostgres "github.com/buildset/buildset/auth/postgres"
	authsqlite "github.com/buildset/buildset/auth/sqlite"
	"github.com/buildset/buildset/authz"
	authzpostgres "github.com/buildset/buildset/authz/postgres"
	authzsqlite "github.com/buildset/buildset/authz/sqlite"
	"github.com/buildset/buildset/content"
	contentpostgres "github.com/buildset/buildset/content/postgres"
	contentsqlite "github.com/buildset/buildset/content/sqlite"
)

// administratorRole is the application's vocabulary; authz only stores the string.
const administratorRole = "admin"

// services holds every service this binary runs. Each is built from its own backend and never sees
// the others' packages.
type services struct {
	auth    *auth.Service
	authz   *authz.Service
	content *content.Service
}

func newServices(
	ctx context.Context,
	cfg *Config,
	stores *Stores,
	logger *slog.Logger,
) (*services, error) {
	bcryptAlgorithm, err := hash.NewBcrypt(cfg.Auth.BcryptCost)
	if err != nil {
		return nil, fmt.Errorf("build bcrypt algorithm: %w", err)
	}

	// TODO: register argon2id here and prefer it once implemented. Existing bcrypt hashes keep
	// verifying, and each user is upgraded on their next login.
	passwords, err := hash.NewRegistry(bcryptAlgorithm)
	if err != nil {
		return nil, fmt.Errorf("build password registry: %w", err)
	}

	authRepository, err := newAuthRepository(ctx, cfg.Database.Driver, stores.Auth)
	if err != nil {
		return nil, fmt.Errorf("build auth repository: %w", err)
	}

	authzRepository, err := newAuthzRepository(ctx, cfg.Database.Driver, stores.Authz)
	if err != nil {
		return nil, fmt.Errorf("build authz repository: %w", err)
	}

	authzService := authz.NewService(authzRepository)

	// The hook lets auth create the first administrator without knowing that authz exists.
	firstUserHook := auth.FirstUserHookFunc(func(ctx context.Context, userRef string) error {
		if err := authzService.AssignRole(ctx, userRef, administratorRole); err != nil {
			return fmt.Errorf("assign %s role to %s: %w", administratorRole, userRef, err)
		}

		logger.InfoContext(
			ctx,
			"first user is now an administrator",
			slog.String("user_ref", userRef),
		)

		return nil
	})

	authService, err := auth.NewService(
		authRepository,
		passwords,
		firstUserHook,
		cfg.Auth.SessionTTL,
		logger,
	)
	if err != nil {
		return nil, fmt.Errorf("build auth service: %w", err)
	}

	contentRepository, err := newContentRepository(ctx, cfg.Database.Driver, stores.Content)
	if err != nil {
		return nil, fmt.Errorf("build content repository: %w", err)
	}

	return &services{
		auth:    authService,
		authz:   authzService,
		content: content.NewService(contentRepository),
	}, nil
}

// The factories below are the only places that name a storage backend.

func newAuthRepository(ctx context.Context, driver string, db *sql.DB) (auth.Repository, error) {
	if driver == DriverPostgres {
		return authpostgres.NewRepository(ctx, db)
	}

	return authsqlite.NewRepository(ctx, db)
}

func newAuthzRepository(ctx context.Context, driver string, db *sql.DB) (authz.Repository, error) {
	if driver == DriverPostgres {
		return authzpostgres.NewRepository(ctx, db)
	}

	return authzsqlite.NewRepository(ctx, db)
}

func newContentRepository(
	ctx context.Context,
	driver string,
	db *sql.DB,
) (content.Repository, error) {
	if driver == DriverPostgres {
		return contentpostgres.NewRepository(ctx, db)
	}

	return contentsqlite.NewRepository(ctx, db)
}
