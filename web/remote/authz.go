package remote

import (
	"context"

	authzclient "github.com/buildset/buildset/authz/client"
	"github.com/buildset/buildset/web"
)

// Authz satisfies web.Authz by calling the authorization service.
type Authz struct {
	client *authzclient.Client
}

var _ web.Authz = (*Authz)(nil)

func NewAuthz(client *authzclient.Client) *Authz {
	return &Authz{client: client}
}

// Can answers false on any failure. The site treats a non-nil error as "do not continue", so a
// failure here can never widen access.
func (a *Authz) Can(ctx context.Context, subject, action, resource string) (bool, error) {
	allowed, err := a.client.Can(ctx, subject, action, resource)

	return allowed, translate(err)
}

func (a *Authz) Grant(
	ctx context.Context,
	subject string,
	actions []string,
	resource string,
) error {
	return translate(a.client.Grant(ctx, subject, actions, resource))
}

func (a *Authz) AssignRole(ctx context.Context, subject, role string) error {
	return translate(a.client.AssignRole(ctx, subject, role))
}

func (a *Authz) RevokeRole(ctx context.Context, subject, role string) error {
	return translate(a.client.RevokeRole(ctx, subject, role))
}

func (a *Authz) SubjectRoles(ctx context.Context, subject string) ([]string, error) {
	roles, err := a.client.SubjectRoles(ctx, subject)
	if err != nil {
		return nil, translate(err)
	}

	return roles, nil
}

func (a *Authz) ListRoles(ctx context.Context) ([]string, error) {
	roles, err := a.client.ListRoles(ctx)
	if err != nil {
		return nil, translate(err)
	}

	return roles, nil
}

func (a *Authz) PurgeResource(ctx context.Context, resource string) error {
	return translate(a.client.PurgeResource(ctx, resource))
}

func (a *Authz) PurgeSubject(ctx context.Context, subject string) error {
	return translate(a.client.PurgeSubject(ctx, subject))
}
