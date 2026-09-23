package authapp

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/buildset/buildset/auth"
	authui "github.com/buildset/buildset/auth/ui"
	authzclient "github.com/buildset/buildset/authz/client"
	"github.com/buildset/buildset/pkg/action"
)

// The first user's role assignment is the one call in this binary that is retried: it is
// idempotent, happens once per installation, and losing it leaves an instance nobody can
// administer. The long timeout is for the same reason: a cold authorization service is worth it.
const (
	firstUserTimeout = 10 * time.Second
	firstUserRetries = 2
)

// firstUserHook makes the first account an administrator. The call crosses the network here, so
// setup depends on the authorization service being up, which is why compose starts it first.
func firstUserHook(
	client *authzclient.Client,
	role string,
	logger *slog.Logger,
) auth.FirstUserHook {
	return auth.FirstUserHookFunc(func(ctx context.Context, userRef string) error {
		var err error

		for attempt := range firstUserRetries {
			attemptCtx, cancel := context.WithTimeout(ctx, firstUserTimeout)
			err = client.AssignRole(attemptCtx, userRef, role)
			cancel()

			if err == nil {
				logger.InfoContext(ctx, "first user is now an administrator",
					slog.String("user_ref", userRef),
					slog.String("role", role),
				)

				return nil
			}

			logger.WarnContext(ctx, "assign first user role",
				slog.String("user_ref", userRef),
				slog.Int("attempt", attempt+1),
				slog.Any("error", err),
			)
		}

		// The account is about to be rolled back. An assignment that landed after the call gave up
		// leaves a role row naming an account that no longer exists. Identifiers are not reused, so
		// it can never match a future account; it is logged rather than compensated for with another
		// call that could fail the same way.
		logger.ErrorContext(ctx, "first user rolled back after an uncertain role assignment",
			slog.String("user_ref", userRef),
			slog.String("role", role),
		)

		return fmt.Errorf("assign %s role to %s: %w", role, userRef, err)
	})
}

// registrationPolicy asks the authorization service who may reach the registration form. Public
// sign-up is a configuration switch; otherwise it takes a permission.
func registrationPolicy(open bool, client *authzclient.Client) authui.RegistrationPolicy {
	return authui.RegistrationPolicyFunc(func(ctx context.Context, actorRef string) (bool, error) {
		if open {
			return true, nil
		}

		if actorRef == "" {
			return false, nil
		}

		return client.Can(ctx, actorRef, action.UserCreate, action.AnyUser)
	})
}
