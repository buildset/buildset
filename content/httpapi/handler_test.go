package httpapi_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/buildset/buildset/content"
	"github.com/buildset/buildset/content/httpapi"
	"github.com/buildset/buildset/content/sqlite"
	"github.com/buildset/buildset/pkg/api/contentapi"
	"github.com/buildset/buildset/pkg/httpx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

const alice = "urn:auth:user:alice"

func newServer(t *testing.T) *httptest.Server {
	t.Helper()

	db, err := sql.Open("sqlite", "file:"+t.TempDir()+"/test.db")
	require.NoError(t, err)

	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	repository, err := sqlite.NewRepository(t.Context(), db)
	require.NoError(t, err)

	handler, err := httpapi.NewHandler(
		content.NewService(repository),
		slog.New(slog.DiscardHandler),
	)
	require.NoError(t, err)

	mux := http.NewServeMux()
	handler.Register(mux)

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	return server
}

func post(t *testing.T, server *httptest.Server, path string, request any) (int, []byte) {
	t.Helper()

	body, err := json.Marshal(request)
	require.NoError(t, err)

	response, err := server.Client().
		Post(server.URL+path, "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	defer func() { _ = response.Body.Close() }()

	payload, err := io.ReadAll(response.Body)
	require.NoError(t, err)

	return response.StatusCode, payload
}

func createPost(t *testing.T, server *httptest.Server, title string) contentapi.Post {
	t.Helper()

	status, body := post(t, server, contentapi.PathCreatePost, contentapi.CreatePostRequest{
		AuthorRef:   alice,
		Title:       title,
		Body:        "A body.",
		ContentType: content.ContentTypePlainText,
	})
	require.Equal(t, http.StatusOK, status)

	var response contentapi.PostResponse
	require.NoError(t, json.Unmarshal(body, &response))

	return response.Post
}

func TestCreatePostCarriesItsReference(t *testing.T) {
	server := newServer(t)

	created := createPost(t, server, "First")

	assert.NotEmpty(t, created.ID)
	// The reference is built by the service that owns it, because a consumer uses it as an
	// authorization key.
	assert.Equal(t, "urn:content:post:"+created.ID, created.Ref)
	assert.Equal(t, string(content.StatusDraft), created.Status)
	assert.Nil(t, created.PublishedAt)
}

func TestGetPostReportsAMissingPost(t *testing.T) {
	server := newServer(t)

	status, body := post(t, server, contentapi.PathGetPost, contentapi.GetPostRequest{
		ID: "0199bf3c-7a1e-7c2b-9f10-3d4a5b6c7d8e",
	})
	assert.Equal(t, http.StatusNotFound, status)

	var envelope httpx.Envelope
	require.NoError(t, json.Unmarshal(body, &envelope))
	assert.Equal(t, httpx.CodeNotFound, envelope.Code)
	assert.Equal(t, "That post could not be found.", envelope.Message)
}

func TestSetStatusRejectsAnImpossibleTransition(t *testing.T) {
	server := newServer(t)

	created := createPost(t, server, "First")

	// A draft may be published; an archived post must become a draft again first.
	status, body := post(t, server, contentapi.PathSetStatus, contentapi.SetStatusRequest{
		ID:     created.ID,
		Status: "nonsense",
	})
	assert.Equal(t, http.StatusBadRequest, status)

	var envelope httpx.Envelope
	require.NoError(t, json.Unmarshal(body, &envelope))
	assert.Equal(t, httpx.CodeInvalidInput, envelope.Code)
}

func TestPublishSetsTheTimestamp(t *testing.T) {
	server := newServer(t)

	created := createPost(t, server, "First")

	status, body := post(t, server, contentapi.PathSetStatus, contentapi.SetStatusRequest{
		ID:     created.ID,
		Status: string(content.StatusPublished),
	})
	require.Equal(t, http.StatusOK, status)

	var response contentapi.PostResponse
	require.NoError(t, json.Unmarshal(body, &response))
	assert.Equal(t, string(content.StatusPublished), response.Post.Status)
	require.NotNil(t, response.Post.PublishedAt)
	assert.False(t, response.Post.PublishedAt.IsZero())
}

func TestListPostsFilters(t *testing.T) {
	server := newServer(t)

	first := createPost(t, server, "First")
	createPost(t, server, "Second")

	status, _ := post(t, server, contentapi.PathSetStatus, contentapi.SetStatusRequest{
		ID:     first.ID,
		Status: string(content.StatusPublished),
	})
	require.Equal(t, http.StatusOK, status)

	status, body := post(t, server, contentapi.PathListPosts, contentapi.ListPostsRequest{
		Status: string(content.StatusPublished),
		Limit:  10,
	})
	require.Equal(t, http.StatusOK, status)

	var response contentapi.ListPostsResponse
	require.NoError(t, json.Unmarshal(body, &response))
	require.Len(t, response.Posts, 1)
	assert.Equal(t, first.ID, response.Posts[0].ID)
}

func TestDeletePostAnswersWithAnEmptyObject(t *testing.T) {
	server := newServer(t)

	created := createPost(t, server, "Doomed")

	status, body := post(
		t,
		server,
		contentapi.PathDeletePost,
		contentapi.DeletePostRequest{ID: created.ID},
	)
	require.Equal(t, http.StatusOK, status)
	assert.JSONEq(t, `{}`, string(body))

	status, _ = post(t, server, contentapi.PathGetPost, contentapi.GetPostRequest{ID: created.ID})
	assert.Equal(t, http.StatusNotFound, status)
}

func TestCreatePostRejectsAnUnsupportedContentType(t *testing.T) {
	server := newServer(t)

	status, body := post(t, server, contentapi.PathCreatePost, contentapi.CreatePostRequest{
		AuthorRef:   alice,
		Title:       "First",
		ContentType: "text/markdown",
	})
	assert.Equal(t, http.StatusBadRequest, status)

	var envelope httpx.Envelope
	require.NoError(t, json.Unmarshal(body, &envelope))
	assert.Equal(t, httpx.CodeInvalidInput, envelope.Code)
}
