package auth

import "context"

// FirstUserHook runs once, when setup creates the very first user, so the composition root can make
// that account an administrator without auth knowing an authorization service exists.
type FirstUserHook interface {
	OnFirstUser(ctx context.Context, userRef string) error
}

type FirstUserHookFunc func(ctx context.Context, userRef string) error

func (f FirstUserHookFunc) OnFirstUser(ctx context.Context, userRef string) error {
	return f(ctx, userRef)
}
