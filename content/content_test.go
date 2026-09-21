package content_test

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/buildset/buildset/content"
	contentsqlite "github.com/buildset/buildset/content/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

const (
	adaRef   = "urn:auth:user:0199bf3c-7a1e-7c2b-9f10-000000000001"
	graceRef = "urn:auth:user:0199bf3c-7a1e-7c2b-9f10-000000000002"
)

func newService(t *testing.T) *content.Service {
	t.Helper()

	db, err := sql.Open("sqlite", "file:"+t.Name()+"?mode=memory&cache=shared")
	require.NoError(t, err)

	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })

	repository, err := contentsqlite.NewRepository(t.Context(), db)
	require.NoError(t, err)

	return content.NewService(repository)
}

func createPost(t *testing.T, service *content.Service, authorRef, title string) *content.Post {
	t.Helper()

	post, err := service.CreatePost(t.Context(), content.CreatePostRequest{
		AuthorRef: authorRef,
		Title:     title,
		Body:      "Hello.",
	})
	require.NoError(t, err)

	return post
}

func TestCreatePost(t *testing.T) {
	service := newService(t)

	post := createPost(t, service, adaRef, "  On Computing  ")

	assert.Equal(t, content.StatusDraft, post.Status, "a new post is never published by accident")
	assert.Equal(t, "On Computing", post.Title, "the title is trimmed")
	assert.Equal(t, content.ContentTypePlainText, post.ContentType)
	assert.Nil(t, post.PublishedAt)
	assert.Equal(t, "urn:content:post:"+post.ID, post.Ref())

	stored, err := service.GetPostByRef(t.Context(), post.Ref())
	require.NoError(t, err)
	assert.Equal(t, post.ID, stored.ID)
}

func TestCreatePostRejectsBadInput(t *testing.T) {
	service := newService(t)
	ctx := t.Context()

	_, err := service.CreatePost(ctx, content.CreatePostRequest{AuthorRef: "not-a-reference", Title: "Title"})
	require.ErrorIs(t, err, content.ErrInvalidPost)

	_, err = service.CreatePost(ctx, content.CreatePostRequest{AuthorRef: adaRef, Title: "   "})
	require.ErrorIs(t, err, content.ErrInvalidPost)

	_, err = service.CreatePost(ctx, content.CreatePostRequest{AuthorRef: adaRef, Title: strings.Repeat("a", content.MaxTitleLength+1)})
	require.ErrorIs(t, err, content.ErrInvalidPost)

	_, err = service.CreatePost(ctx, content.CreatePostRequest{AuthorRef: adaRef, Title: "Title", ContentType: "text/markdown"})
	require.ErrorIs(t, err, content.ErrUnsupportedContent)
}

func TestStatusLifecycle(t *testing.T) {
	service := newService(t)
	ctx := t.Context()

	post := createPost(t, service, adaRef, "On Computing")

	published, err := service.SetStatus(ctx, post.ID, content.StatusPublished)
	require.NoError(t, err)
	require.NotNil(t, published.PublishedAt)
	firstPublication := *published.PublishedAt

	archived, err := service.SetStatus(ctx, post.ID, content.StatusArchived)
	require.NoError(t, err)
	assert.Equal(t, content.StatusArchived, archived.Status)

	// An archived post comes back as a draft, so republishing is deliberate.
	_, err = service.SetStatus(ctx, post.ID, content.StatusPublished)
	require.ErrorIs(t, err, content.ErrInvalidTransition)

	draft, err := service.SetStatus(ctx, post.ID, content.StatusDraft)
	require.NoError(t, err)
	assert.Equal(t, content.StatusDraft, draft.Status)

	republished, err := service.SetStatus(ctx, post.ID, content.StatusPublished)
	require.NoError(t, err)
	assert.Equal(t, firstPublication, *republished.PublishedAt, "the original publication date must not move")

	_, err = service.SetStatus(ctx, post.ID, content.Status("deleted"))
	require.ErrorIs(t, err, content.ErrInvalidStatus)
}

func TestListPostsFilters(t *testing.T) {
	service := newService(t)
	ctx := t.Context()

	adaDraft := createPost(t, service, adaRef, "Ada draft")
	adaPost := createPost(t, service, adaRef, "Ada published")
	createPost(t, service, graceRef, "Grace draft")

	_, err := service.SetStatus(ctx, adaPost.ID, content.StatusPublished)
	require.NoError(t, err)

	published, err := service.ListPosts(ctx, content.PostFilter{Status: content.StatusPublished})
	require.NoError(t, err)
	require.Len(t, published, 1)
	assert.Equal(t, adaPost.ID, published[0].ID)

	byAda, err := service.ListPosts(ctx, content.PostFilter{AuthorRef: adaRef})
	require.NoError(t, err)
	require.Len(t, byAda, 2)

	adaDrafts, err := service.ListPosts(ctx, content.PostFilter{AuthorRef: adaRef, Status: content.StatusDraft})
	require.NoError(t, err)
	require.Len(t, adaDrafts, 1)
	assert.Equal(t, adaDraft.ID, adaDrafts[0].ID)

	all, err := service.ListPosts(ctx, content.PostFilter{})
	require.NoError(t, err)
	assert.Len(t, all, 3)
}

func TestDeletePost(t *testing.T) {
	service := newService(t)
	ctx := t.Context()

	post := createPost(t, service, adaRef, "On Computing")

	require.NoError(t, service.DeletePost(ctx, post.ID))
	require.ErrorIs(t, service.DeletePost(ctx, post.ID), content.ErrPostNotFound)

	_, err := service.GetPost(ctx, post.ID)
	require.ErrorIs(t, err, content.ErrPostNotFound)
}

func TestRenderHTML(t *testing.T) {
	t.Parallel()

	rendered, err := content.RenderHTML(content.ContentTypePlainText, "First line.\nSecond line.\n\nNew paragraph.")
	require.NoError(t, err)
	assert.Equal(t, "<p>First line.<br>Second line.</p><p>New paragraph.</p>", string(rendered))

	// A body is data, never markup.
	rendered, err = content.RenderHTML(content.ContentTypePlainText, `<script>alert("x")</script>`)
	require.NoError(t, err)
	assert.Equal(t, `<p>&lt;script&gt;alert(&#34;x&#34;)&lt;/script&gt;</p>`, string(rendered))

	_, err = content.RenderHTML("text/html", "<b>no</b>")
	require.ErrorIs(t, err, content.ErrUnsupportedContent)
}
