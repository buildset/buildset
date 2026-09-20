package auth

import "context"

// FirstUserHook runs once, when setup creates the very first user. It exists so the first account
// can be made an administrator without auth knowing that an authorization service exists. The
// composition root supplies the implementation.
type FirstUserHook interface {
	OnFirstUser(ctx context.Context, userRef string) error
}

type FirstUserHookFunc func(ctx context.Context, userRef string) error

func (f FirstUserHookFunc) OnFirstUser(ctx context.Context, userRef string) error {
	return f(ctx, userRef)
}
