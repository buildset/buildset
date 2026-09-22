package remote

import (
	"context"
	"html/template"

	contentclient "github.com/buildset/buildset/content/client"
	"github.com/buildset/buildset/content/render"
	"github.com/buildset/buildset/pkg/api/contentapi"
	"github.com/buildset/buildset/web"
)

// Content satisfies web.Content by calling the content service.
type Content struct {
	client *contentclient.Client
}

var _ web.Content = (*Content)(nil)

func NewContent(client *contentclient.Client) *Content {
	return &Content{client: client}
}

func (c *Content) GetPost(ctx context.Context, id string) (*web.Post, error) {
	post, err := c.client.GetPost(ctx, id)
	if err != nil {
		return nil, translate(err)
	}

	return toWebPost(post), nil
}

func (c *Content) ListPosts(ctx context.Context, status, authorRef string, limit int) ([]web.Post, error) {
	posts, err := c.client.ListPosts(ctx, status, authorRef, limit)
	if err != nil {
		return nil, translate(err)
	}

	converted := make([]web.Post, 0, len(posts))
	for _, post := range posts {
		converted = append(converted, *toWebPost(post))
	}

	return converted, nil
}

func (c *Content) CreatePost(ctx context.Context, authorRef, title, body, contentType string) (*web.Post, error) {
	post, err := c.client.CreatePost(ctx, authorRef, title, body, contentType)
	if err != nil {
		return nil, translate(err)
	}

	return toWebPost(post), nil
}

func (c *Content) UpdatePost(ctx context.Context, id, title, body, contentType string) (*web.Post, error) {
	post, err := c.client.UpdatePost(ctx, id, title, body, contentType)
	if err != nil {
		return nil, translate(err)
	}

	return toWebPost(post), nil
}

func (c *Content) SetStatus(ctx context.Context, id, status string) (*web.Post, error) {
	post, err := c.client.SetStatus(ctx, id, status)
	if err != nil {
		return nil, translate(err)
	}

	return toWebPost(post), nil
}

func (c *Content) DeletePost(ctx context.Context, id string) error {
	return translate(c.client.DeletePost(ctx, id))
}

// RenderBody is computed here rather than asked for. It is a pure function of the body and its
// content type, and it uses the content service's own rules, so a round trip would buy nothing and
// would make every page wait on another process.
func (c *Content) RenderBody(_ context.Context, post *web.Post) (template.HTML, error) {
	rendered, err := render.HTML(post.ContentType, post.Body)
	if err != nil {
		return "", web.NewError(web.ErrInvalidInput, err.Error())
	}

	return rendered, nil
}

func toWebPost(post contentapi.Post) *web.Post {
	return &web.Post{
		Ref:         post.Ref,
		ID:          post.ID,
		AuthorRef:   post.AuthorRef,
		Title:       post.Title,
		Body:        post.Body,
		ContentType: post.ContentType,
		Status:      post.Status,
		CreatedAt:   post.CreatedAt,
		UpdatedAt:   post.UpdatedAt,
		PublishedAt: post.PublishedAt,
	}
}
