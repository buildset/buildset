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

// The first user's role assignment is the one call in this binary that is retried.
//
// It is idempotent, it happens once in the lifetime of an installation, it is not on any visitor's
// critical path, and losing it leaves an instance nobody can administer. Its timeout is longer
// than a normal call for the same reason: this runs while an operator watches the setup form, and
// a cold authorization service is worth waiting for.
const (
	firstUserTimeout = 10 * time.Second
	firstUserRetries = 2
)

// firstUserHook makes the first account an administrator.
//
// In the single binary this is an in-process call. Here it crosses the network, so setup now
// depends on the authorization service being up, which is why compose starts it first.
func firstUserHook(client *authzclient.Client, role string, logger *slog.Logger) auth.FirstUserHook {
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

		// The account is about to be rolled back. If the assignment did in fact land before the
		// call gave up, a role row is left behind naming an account that no longer exists. It can
		// never match a future account because identifiers are not reused, but it is worth being
		// able to find, so it is logged loudly rather than compensated for with another call that
		// could fail the same way.
		logger.ErrorContext(ctx, "first user rolled back after an uncertain role assignment",
			slog.String("user_ref", userRef),
			slog.String("role", role),
		)

		return fmt.Errorf("assign %s role to %s: %w", role, userRef, err)
	})
}

// registrationPolicy decides who may reach the registration form.
//
// Who may create an account is an authorization question, so it is answered by asking the
// authorization service rather than by anything inside this service. Public sign-up is a
// configuration switch; otherwise it takes a permission.
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
