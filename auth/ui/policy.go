package ui

import "context"

// RegistrationPolicy decides who may create an account, so this package can keep the registration
// form while the decision is made elsewhere. actorRef is the signed-in visitor, empty if anonymous.
type RegistrationPolicy interface {
	MayRegister(ctx context.Context, actorRef string) (bool, error)
}

type RegistrationPolicyFunc func(ctx context.Context, actorRef string) (bool, error)

func (f RegistrationPolicyFunc) MayRegister(ctx context.Context, actorRef string) (bool, error) {
	return f(ctx, actorRef)
}
