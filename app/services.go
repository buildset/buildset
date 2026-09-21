package app

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/buildset/buildset/auth"
	"github.com/buildset/buildset/auth/hash"
	authsqlite "github.com/buildset/buildset/auth/sqlite"
	"github.com/buildset/buildset/authz"
	authzsqlite "github.com/buildset/buildset/authz/sqlite"
	"github.com/buildset/buildset/content"
	contentsqlite "github.com/buildset/buildset/content/sqlite"
)

// administratorRole is the role the first user is given. The name lives here because it is the
// application's vocabulary; authz only stores the string.
const administratorRole = "admin"

// services holds every service this binary runs. It is the only place that knows they share a
// process; each service is built from its own backend and never sees the others' packages.
type services struct {
	auth    *auth.Service
	authz   *authz.Service
	content *content.Service
}

func newServices(ctx context.Context, cfg *Config, db *sql.DB, logger *slog.Logger) (*services, error) {
	bcryptAlgorithm, err := hash.NewBcrypt(cfg.Auth.BcryptCost)
	if err != nil {
		return nil, fmt.Errorf("build bcrypt algorithm: %w", err)
	}

	// TODO: register argon2id here and make it preferred once it is implemented. Existing bcrypt
	// hashes keep verifying, and each user is upgraded on their next login.
	passwords, err := hash.NewRegistry(bcryptAlgorithm)
	if err != nil {
		return nil, fmt.Errorf("build password registry: %w", err)
	}

	authRepository, err := authsqlite.NewRepository(ctx, db)
	if err != nil {
		return nil, fmt.Errorf("build auth repository: %w", err)
	}

	authzRepository, err := authzsqlite.NewRepository(ctx, db)
	if err != nil {
		return nil, fmt.Errorf("build authz repository: %w", err)
	}

	authzService := authz.NewService(authzRepository)

	// The hook is the whole reason auth can create the first administrator without knowing that an
	// authorization service exists. Only this file sees both sides.
	firstUserHook := auth.FirstUserHookFunc(func(ctx context.Context, userRef string) error {
		if err := authzService.AssignRole(ctx, userRef, administratorRole); err != nil {
			return fmt.Errorf("assign %s role to %s: %w", administratorRole, userRef, err)
		}

		logger.InfoContext(ctx, "first user is now an administrator", slog.String("user_ref", userRef))

		return nil
	})

	authService, err := auth.NewService(authRepository, passwords, firstUserHook, cfg.Auth.SessionTTL, logger)
	if err != nil {
		return nil, fmt.Errorf("build auth service: %w", err)
	}

	contentRepository, err := contentsqlite.NewRepository(ctx, db)
	if err != nil {
		return nil, fmt.Errorf("build content repository: %w", err)
	}

	return &services{
		auth:    authService,
		authz:   authzService,
		content: content.NewService(contentRepository),
	}, nil
}
