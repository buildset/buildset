package remote_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	authclient "github.com/buildset/buildset/auth/client"
	"github.com/buildset/buildset/pkg/httpx"
	"github.com/buildset/buildset/web"
	"github.com/buildset/buildset/web/remote"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newAuth(t *testing.T, handler http.HandlerFunc) *remote.Auth {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	client, err := authclient.New(server.URL, httpx.ClientOptions{Timeout: 2 * time.Second})
	require.NoError(t, err)

	return remote.NewAuth(client, remote.DefaultURLs())
}

// This is the contract, from the other end. web/session.go signs a visitor out only when it gets
// ErrNotFound, and returns 503 for anything else. If a failure ever produced ErrNotFound, an
// outage would silently sign everybody out.
func TestResolveSessionOnlyReportsNotFoundWhenTheServiceSaidSo(t *testing.T) {
	tests := []struct {
		name        string
		status      int
		contentType string
		body        string
		wantKind    error
	}{
		{
			name:        "the session is unknown",
			status:      http.StatusNotFound,
			contentType: "application/json",
			body:        `{"code":"not_found","message":"That account could not be found."}`,
			wantKind:    web.ErrNotFound,
		},
		{
			name:        "the request was invalid",
			status:      http.StatusBadRequest,
			contentType: "application/json",
			body:        `{"code":"invalid_input","message":"That request could not be read."}`,
			wantKind:    web.ErrInvalidInput,
		},
		{
			name:        "the username is taken",
			status:      http.StatusConflict,
			contentType: "application/json",
			body:        `{"code":"conflict","message":"That username is already taken."}`,
			wantKind:    web.ErrConflict,
		},
		{
			name:        "the identity service broke",
			status:      http.StatusInternalServerError,
			contentType: "text/plain",
			body:        "resolve session failed",
		},
		{
			name:        "the gateway could not reach it",
			status:      http.StatusBadGateway,
			contentType: "text/html",
			body:        "<html>502</html>",
		},
		{
			name:        "it is starting up",
			status:      http.StatusServiceUnavailable,
			contentType: "text/plain",
			body:        "not ready",
		},
		{
			name:        "a proxy answered 404 with its own page",
			status:      http.StatusNotFound,
			contentType: "text/html",
			body:        "<html>404</html>",
		},
		{
			name:        "a future version reported something unknown",
			status:      http.StatusNotFound,
			contentType: "application/json",
			body:        `{"code":"rate_limited","message":"slow down"}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			auth := newAuth(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", test.contentType)
				w.WriteHeader(test.status)
				_, _ = w.Write([]byte(test.body))
			})

			_, err := auth.ResolveSession(t.Context(), "token")
			require.Error(t, err)

			if test.wantKind == nil {
				assert.False(t, errors.Is(err, web.ErrNotFound),
					"a failure must not look like a missing session: %v", err)
				assert.False(t, errors.Is(err, web.ErrInvalidInput))
				assert.False(t, errors.Is(err, web.ErrConflict))

				return
			}

			assert.ErrorIs(t, err, test.wantKind)

			var webErr *web.Error
			require.True(t, errors.As(err, &webErr))
			assert.NotEmpty(
				t,
				webErr.Message,
				"the message shown to a visitor must survive the wire",
			)
		})
	}
}

func TestResolveSessionOnAnUnreachableServiceIsNotNotFound(t *testing.T) {
	client, err := authclient.New("http://127.0.0.1:1", httpx.ClientOptions{Timeout: time.Second})
	require.NoError(t, err)

	_, resolveErr := remote.NewAuth(client, remote.DefaultURLs()).
		ResolveSession(t.Context(), "token")
	require.Error(t, resolveErr)
	assert.False(t, errors.Is(resolveErr, web.ErrNotFound))
}

func TestLoginURLKeepsTheRedirectOnThisSite(t *testing.T) {
	urls := remote.DefaultURLs()

	assert.Equal(t, "/login", urls.LoginURL("/"))
	assert.Equal(t, "/login?next=%2Fadmin%2Fposts", urls.LoginURL("/admin/posts"))
	// Anything that could leave the origin collapses to the home page, so no next is carried.
	assert.Equal(t, "/login", urls.LoginURL("https://evil.example/"))
	assert.Equal(t, "/login", urls.LoginURL("//evil.example/"))
	assert.Equal(t, "/logout", urls.LogoutURL())
	assert.Equal(t, "/password", urls.PasswordURL())
	assert.Equal(t, "/register", urls.RegisterURL())
}
