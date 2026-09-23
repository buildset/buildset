// Package client calls the content service over HTTP. Rendering a body is not here: it is a pure
// function of the body and its content type, so a consumer calls content/render locally.
package client

import (
	"context"

	"github.com/buildset/buildset/pkg/api/contentapi"
	"github.com/buildset/buildset/pkg/httpx"
)

type Client struct {
	call *httpx.Client
}

func New(baseURL string, opts httpx.ClientOptions) (*Client, error) {
	call, err := httpx.NewClient("content", baseURL, opts)
	if err != nil {
		return nil, err
	}

	return &Client{call: call}, nil
}

func (c *Client) GetPost(ctx context.Context, id string) (contentapi.Post, error) {
	return c.post(ctx, contentapi.PathGetPost, contentapi.GetPostRequest{ID: id})
}

func (c *Client) ListPosts(ctx context.Context, status, authorRef string, limit int) ([]contentapi.Post, error) {
	var response contentapi.ListPostsResponse

	err := c.call.Call(ctx, contentapi.PathListPosts, contentapi.ListPostsRequest{
		Status:    status,
		AuthorRef: authorRef,
		Limit:     limit,
	}, &response)
	if err != nil {
		return nil, err
	}

	return response.Posts, nil
}

func (c *Client) CreatePost(ctx context.Context, authorRef, title, body, contentType string) (contentapi.Post, error) {
	return c.post(ctx, contentapi.PathCreatePost, contentapi.CreatePostRequest{
		AuthorRef:   authorRef,
		Title:       title,
		Body:        body,
		ContentType: contentType,
	})
}

func (c *Client) UpdatePost(ctx context.Context, id, title, body, contentType string) (contentapi.Post, error) {
	return c.post(ctx, contentapi.PathUpdatePost, contentapi.UpdatePostRequest{
		ID:          id,
		Title:       title,
		Body:        body,
		ContentType: contentType,
	})
}

func (c *Client) SetStatus(ctx context.Context, id, status string) (contentapi.Post, error) {
	return c.post(ctx, contentapi.PathSetStatus, contentapi.SetStatusRequest{ID: id, Status: status})
}

func (c *Client) DeletePost(ctx context.Context, id string) error {
	return c.call.Call(ctx, contentapi.PathDeletePost, contentapi.DeletePostRequest{ID: id}, nil)
}

func (c *Client) post(ctx context.Context, path string, request any) (contentapi.Post, error) {
	var response contentapi.PostResponse

	if err := c.call.Call(ctx, path, request, &response); err != nil {
		return contentapi.Post{}, err
	}

	return response.Post, nil
}
