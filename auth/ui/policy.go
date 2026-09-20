package ui

import "context"

// RegistrationPolicy decides who may create an account. It exists so this package can keep the
// registration form, which handles a password, while the decision about who is allowed to use it
// is made elsewhere. The composition root supplies the implementation, and auth stays unaware that
// an authorization service exists.
//
// actorRef is the signed-in visitor, or empty for an anonymous one.
type RegistrationPolicy interface {
	MayRegister(ctx context.Context, actorRef string) (bool, error)
}

type RegistrationPolicyFunc func(ctx context.Context, actorRef string) (bool, error)

func (f RegistrationPolicyFunc) MayRegister(ctx context.Context, actorRef string) (bool, error) {
	return f(ctx, actorRef)
}
