// Package client calls the authorization service over HTTP. It returns the wire types and httpx
// errors, and knows nothing about any consumer.
package client

import (
	"context"

	"github.com/buildset/buildset/pkg/api/authzapi"
	"github.com/buildset/buildset/pkg/httpx"
)

type Client struct {
	call *httpx.Client
}

func New(baseURL string, opts httpx.ClientOptions) (*Client, error) {
	call, err := httpx.NewClient("authz", baseURL, opts)
	if err != nil {
		return nil, err
	}

	return &Client{call: call}, nil
}

func (c *Client) Can(ctx context.Context, subject, action, resource string) (bool, error) {
	var response authzapi.CanResponse

	err := c.call.Call(ctx, authzapi.PathCan, authzapi.CanRequest{
		Subject:  subject,
		Action:   action,
		Resource: resource,
	}, &response)
	if err != nil {
		// Never "allowed" on a failure. The caller decides what to do, but it is never this.
		return false, err
	}

	return response.Allowed, nil
}

func (c *Client) Grant(
	ctx context.Context,
	subject string,
	actions []string,
	resource string,
) error {
	return c.call.Call(ctx, authzapi.PathGrant, authzapi.GrantRequest{
		Subject:  subject,
		Actions:  actions,
		Resource: resource,
	}, nil)
}

func (c *Client) AssignRole(ctx context.Context, subject, role string) error {
	return c.call.Call(
		ctx,
		authzapi.PathAssignRole,
		authzapi.RoleRequest{Subject: subject, Role: role},
		nil,
	)
}

func (c *Client) RevokeRole(ctx context.Context, subject, role string) error {
	return c.call.Call(
		ctx,
		authzapi.PathRevokeRole,
		authzapi.RoleRequest{Subject: subject, Role: role},
		nil,
	)
}

func (c *Client) SubjectRoles(ctx context.Context, subject string) ([]string, error) {
	var response authzapi.RolesResponse

	if err := c.call.Call(
		ctx,
		authzapi.PathSubjectRoles,
		authzapi.SubjectRequest{Subject: subject},
		&response,
	); err != nil {
		return nil, err
	}

	return response.Roles, nil
}

func (c *Client) ListRoles(ctx context.Context) ([]string, error) {
	var response authzapi.RolesResponse

	if err := c.call.Call(ctx, authzapi.PathListRoles, authzapi.Empty{}, &response); err != nil {
		return nil, err
	}

	return response.Roles, nil
}

func (c *Client) PurgeResource(ctx context.Context, resource string) error {
	return c.call.Call(
		ctx,
		authzapi.PathPurgeResource,
		authzapi.ResourceRequest{Resource: resource},
		nil,
	)
}

func (c *Client) PurgeSubject(ctx context.Context, subject string) error {
	return c.call.Call(
		ctx,
		authzapi.PathPurgeSubject,
		authzapi.SubjectRequest{Subject: subject},
		nil,
	)
}
