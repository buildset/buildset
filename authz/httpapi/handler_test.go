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

	"github.com/buildset/buildset/authz"
	"github.com/buildset/buildset/authz/httpapi"
	"github.com/buildset/buildset/authz/sqlite"
	"github.com/buildset/buildset/pkg/api/authzapi"
	"github.com/buildset/buildset/pkg/httpx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

const alice = "urn:auth:user:alice"

func newServer(t *testing.T) *httptest.Server {
	t.Helper()

	db, err := sql.Open("sqlite", "file:"+t.TempDir()+"/test.db?_pragma=foreign_keys(ON)")
	require.NoError(t, err)

	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	repository, err := sqlite.NewRepository(t.Context(), db)
	require.NoError(t, err)

	handler, err := httpapi.NewHandler(authz.NewService(repository), slog.New(slog.DiscardHandler))
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

	httpRequest, err := http.NewRequestWithContext(
		t.Context(),
		http.MethodPost,
		server.URL+path,
		bytes.NewReader(body),
	)
	require.NoError(t, err)

	httpRequest.Header.Set("Content-Type", "application/json")

	response, err := server.Client().Do(httpRequest)
	require.NoError(t, err)
	defer func() { _ = response.Body.Close() }()

	payload, err := io.ReadAll(response.Body)
	require.NoError(t, err)

	return response.StatusCode, payload
}

func TestCan(t *testing.T) {
	server := newServer(t)

	status, _ := post(
		t,
		server,
		authzapi.PathAssignRole,
		authzapi.RoleRequest{Subject: alice, Role: "author"},
	)
	require.Equal(t, http.StatusOK, status)

	status, body := post(t, server, authzapi.PathCan, authzapi.CanRequest{
		Subject:  alice,
		Action:   "post.create",
		Resource: "urn:content:post:1",
	})
	require.Equal(t, http.StatusOK, status)

	var allowed authzapi.CanResponse
	require.NoError(t, json.Unmarshal(body, &allowed))
	assert.True(t, allowed.Allowed)

	status, body = post(t, server, authzapi.PathCan, authzapi.CanRequest{
		Subject:  alice,
		Action:   "user.delete",
		Resource: "urn:auth:user:bob",
	})
	require.Equal(t, http.StatusOK, status)
	require.NoError(t, json.Unmarshal(body, &allowed))
	assert.False(t, allowed.Allowed)
}

// An unknown role used to reach the site untranslated and render as an internal error, telling a
// visitor the system had broken when their request had.
func TestAssignRoleRejectsAnUnknownRole(t *testing.T) {
	server := newServer(t)

	status, body := post(
		t,
		server,
		authzapi.PathAssignRole,
		authzapi.RoleRequest{Subject: alice, Role: "wizard"},
	)
	assert.Equal(t, http.StatusBadRequest, status)

	var envelope httpx.Envelope
	require.NoError(t, json.Unmarshal(body, &envelope))
	assert.Equal(t, httpx.CodeInvalidInput, envelope.Code)
	assert.NotEmpty(t, envelope.Message)
}

func TestListRolesReturnsTheSeededRoles(t *testing.T) {
	server := newServer(t)

	status, body := post(t, server, authzapi.PathListRoles, authzapi.Empty{})
	require.Equal(t, http.StatusOK, status)

	var roles authzapi.RolesResponse
	require.NoError(t, json.Unmarshal(body, &roles))
	assert.ElementsMatch(t, []string{"admin", "author", "reader"}, roles.Roles)
}

func TestGrantAndPurgeRoundTrip(t *testing.T) {
	server := newServer(t)

	const resource = "urn:content:post:1"

	status, _ := post(t, server, authzapi.PathGrant, authzapi.GrantRequest{
		Subject:  alice,
		Actions:  []string{"post.update"},
		Resource: resource,
	})
	require.Equal(t, http.StatusOK, status)

	status, body := post(t, server, authzapi.PathCan, authzapi.CanRequest{
		Subject:  alice,
		Action:   "post.update",
		Resource: resource,
	})
	require.Equal(t, http.StatusOK, status)

	var allowed authzapi.CanResponse
	require.NoError(t, json.Unmarshal(body, &allowed))
	require.True(t, allowed.Allowed)

	status, _ = post(
		t,
		server,
		authzapi.PathPurgeResource,
		authzapi.ResourceRequest{Resource: resource},
	)
	require.Equal(t, http.StatusOK, status)

	status, body = post(t, server, authzapi.PathCan, authzapi.CanRequest{
		Subject:  alice,
		Action:   "post.update",
		Resource: resource,
	})
	require.Equal(t, http.StatusOK, status)
	require.NoError(t, json.Unmarshal(body, &allowed))
	assert.False(t, allowed.Allowed)
}

func TestUnreadableBodyIsTheCallersFault(t *testing.T) {
	server := newServer(t)

	httpRequest, err := http.NewRequestWithContext(
		t.Context(),
		http.MethodPost,
		server.URL+authzapi.PathCan,
		bytes.NewReader([]byte("{not json")),
	)
	require.NoError(t, err)

	httpRequest.Header.Set("Content-Type", "application/json")

	response, err := server.Client().Do(httpRequest)
	require.NoError(t, err)
	defer func() { _ = response.Body.Close() }()

	assert.Equal(t, http.StatusBadRequest, response.StatusCode)
}
