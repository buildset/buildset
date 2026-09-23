// Package client calls the identity service over HTTP. Only the operations another service needs
// are here; registering, authenticating, session creation and password change have no endpoint.
package client

import (
	"context"

	"github.com/buildset/buildset/pkg/api/authapi"
	"github.com/buildset/buildset/pkg/httpx"
)

type Client struct {
	call *httpx.Client
}

func New(baseURL string, opts httpx.ClientOptions) (*Client, error) {
	call, err := httpx.NewClient("auth", baseURL, opts)
	if err != nil {
		return nil, err
	}

	return &Client{call: call}, nil
}

// The request body carries a live session token: never log it, and never move it into the path or
// a query string, where an access log would keep it.
func (c *Client) ResolveSession(ctx context.Context, token string) (authapi.User, error) {
	return c.user(ctx, authapi.PathResolveSession, authapi.ResolveSessionRequest{Token: token})
}

func (c *Client) GetUserByRef(ctx context.Context, userRef string) (authapi.User, error) {
	return c.user(ctx, authapi.PathGetUser, authapi.GetUserRequest{UserRef: userRef})
}

func (c *Client) ListUsers(ctx context.Context, limit int) ([]authapi.User, error) {
	var response authapi.ListUsersResponse

	if err := c.call.Call(
		ctx,
		authapi.PathListUsers,
		authapi.ListUsersRequest{Limit: limit},
		&response,
	); err != nil {
		return nil, err
	}

	return response.Users, nil
}

func (c *Client) UpdateProfile(
	ctx context.Context,
	userRef, username, name string,
) (authapi.User, error) {
	return c.user(ctx, authapi.PathUpdateProfile, authapi.UpdateProfileRequest{
		UserRef:  userRef,
		Username: username,
		Name:     name,
	})
}

func (c *Client) DeleteUser(ctx context.Context, userRef string) error {
	return c.call.Call(
		ctx,
		authapi.PathDeleteUser,
		authapi.DeleteUserRequest{UserRef: userRef},
		nil,
	)
}

func (c *Client) SetupOpen(ctx context.Context) (bool, error) {
	var response authapi.SetupOpenResponse

	if err := c.call.Call(ctx, authapi.PathSetupOpen, authapi.Empty{}, &response); err != nil {
		return false, err
	}

	return response.Open, nil
}

func (c *Client) user(ctx context.Context, path string, request any) (authapi.User, error) {
	var response authapi.UserResponse

	if err := c.call.Call(ctx, path, request, &response); err != nil {
		return authapi.User{}, err
	}

	return response.User, nil
}
