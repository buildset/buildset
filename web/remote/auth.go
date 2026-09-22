package remote

import (
	"context"

	authclient "github.com/buildset/buildset/auth/client"
	"github.com/buildset/buildset/pkg/api/authapi"
	"github.com/buildset/buildset/web"
)

// Auth satisfies web.Auth by calling the identity service.
type Auth struct {
	client *authclient.Client
	urls   URLs
}

var _ web.Auth = (*Auth)(nil)

func NewAuth(client *authclient.Client, urls URLs) *Auth {
	return &Auth{client: client, urls: urls}
}

func (a *Auth) ResolveSession(ctx context.Context, token string) (*web.User, error) {
	user, err := a.client.ResolveSession(ctx, token)
	if err != nil {
		return nil, translate(err)
	}

	return toWebUser(user), nil
}

func (a *Auth) GetUserByRef(ctx context.Context, userRef string) (*web.User, error) {
	user, err := a.client.GetUserByRef(ctx, userRef)
	if err != nil {
		return nil, translate(err)
	}

	return toWebUser(user), nil
}

func (a *Auth) ListUsers(ctx context.Context, limit int) ([]web.User, error) {
	users, err := a.client.ListUsers(ctx, limit)
	if err != nil {
		return nil, translate(err)
	}

	converted := make([]web.User, 0, len(users))
	for _, user := range users {
		converted = append(converted, *toWebUser(user))
	}

	return converted, nil
}

func (a *Auth) UpdateProfile(ctx context.Context, userRef, username, name string) (*web.User, error) {
	user, err := a.client.UpdateProfile(ctx, userRef, username, name)
	if err != nil {
		return nil, translate(err)
	}

	return toWebUser(user), nil
}

func (a *Auth) DeleteUser(ctx context.Context, userRef string) error {
	return translate(a.client.DeleteUser(ctx, userRef))
}

func (a *Auth) SetupOpen(ctx context.Context) (bool, error) {
	open, err := a.client.SetupOpen(ctx)

	return open, translate(err)
}

func (a *Auth) LoginURL(next string) string { return a.urls.LoginURL(next) }
func (a *Auth) LogoutURL() string           { return a.urls.LogoutURL() }
func (a *Auth) PasswordURL() string         { return a.urls.PasswordURL() }
func (a *Auth) RegisterURL() string         { return a.urls.RegisterURL() }

func toWebUser(user authapi.User) *web.User {
	return &web.User{
		Ref:      user.Ref,
		ID:       user.ID,
		Username: user.Username,
		Name:     user.Name,
	}
}
