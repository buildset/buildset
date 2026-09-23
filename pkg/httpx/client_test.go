package httpx_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/buildset/buildset/pkg/httpx"
	"github.com/buildset/buildset/pkg/reqid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type echoRequest struct {
	Value string `json:"value"`
}

type echoResponse struct {
	Value string `json:"value"`
}

func newClient(t *testing.T, handler http.HandlerFunc) *httpx.Client {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	client, err := httpx.NewClient(
		"test",
		server.URL,
		httpx.ClientOptions{Timeout: 2 * time.Second},
	)
	require.NoError(t, err)

	return client
}

func TestCallDecodesASuccess(t *testing.T) {
	client := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/v1/echo", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		httpx.WriteJSON(w, echoResponse{Value: "pong"})
	})

	var response echoResponse
	require.NoError(t, client.Call(t.Context(), "/v1/echo", echoRequest{Value: "ping"}, &response))
	assert.Equal(t, "pong", response.Value)
}

// This is the contract. web/session.go refuses to treat a visitor as anonymous unless the identity
// service positively said "no such session"; every other outcome must stay a failure, or a service
// being down silently signs everybody out.
func TestCallOnlyReportsADomainErrorForAWellFormedEnvelope(t *testing.T) {
	tests := []struct {
		name        string
		status      int
		contentType string
		body        string
		wantCode    httpx.Code
	}{
		{
			name:        "not found",
			status:      http.StatusNotFound,
			contentType: "application/json",
			body:        `{"code":"not_found","message":"That account could not be found."}`,
			wantCode:    httpx.CodeNotFound,
		},
		{
			name:        "invalid input",
			status:      http.StatusBadRequest,
			contentType: "application/json",
			body:        `{"code":"invalid_input","message":"That username is not valid."}`,
			wantCode:    httpx.CodeInvalidInput,
		},
		{
			name:        "conflict",
			status:      http.StatusConflict,
			contentType: "application/json",
			body:        `{"code":"conflict","message":"That username is already taken."}`,
			wantCode:    httpx.CodeConflict,
		},
		{
			name:        "charset on the content type is still json",
			status:      http.StatusNotFound,
			contentType: "application/json; charset=utf-8",
			body:        `{"code":"not_found","message":"Gone."}`,
			wantCode:    httpx.CodeNotFound,
		},
		{
			// The service itself failed. Nothing is known about the request.
			name:        "internal server error",
			status:      http.StatusInternalServerError,
			contentType: "application/json",
			body:        `{"code":"not_found","message":"lying about itself"}`,
		},
		{
			// A gateway answering for a service that is not there.
			name:        "bad gateway with an html body",
			status:      http.StatusBadGateway,
			contentType: "text/html",
			body:        "<html><body>502 Bad Gateway</body></html>",
		},
		{
			name:        "service unavailable",
			status:      http.StatusServiceUnavailable,
			contentType: "application/json",
			body:        `{"code":"not_found","message":"still lying"}`,
		},
		{
			// A proxy error page served with the right status but the wrong type.
			name:        "domain status with an html body",
			status:      http.StatusNotFound,
			contentType: "text/html",
			body:        "<html><body>404</body></html>",
		},
		{
			name:        "domain status with an undecodable body",
			status:      http.StatusNotFound,
			contentType: "application/json",
			body:        "{not json",
		},
		{
			// A future version of the service reporting something this client cannot act on.
			name:        "unknown code",
			status:      http.StatusNotFound,
			contentType: "application/json",
			body:        `{"code":"rate_limited","message":"slow down"}`,
		},
		{
			name:        "empty code",
			status:      http.StatusNotFound,
			contentType: "application/json",
			body:        `{"message":"no code at all"}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := newClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", test.contentType)
				w.WriteHeader(test.status)
				_, _ = w.Write([]byte(test.body))
			})

			err := client.Call(t.Context(), "/v1/echo", echoRequest{}, &echoResponse{})
			require.Error(t, err)

			var domain *httpx.Error

			if test.wantCode == "" {
				assert.False(t, errors.As(err, &domain),
					"a failure of the system must not decode as a domain error: %v", err)

				return
			}

			require.True(t, errors.As(err, &domain), "want a domain error, got %v", err)
			assert.Equal(t, test.wantCode, domain.Code)
			assert.NotEmpty(t, domain.Message)
		})
	}
}

func TestCallReportsAnUnreachableServiceAsAFailure(t *testing.T) {
	// A port nothing is listening on stands in for a dependency that is down.
	client, err := httpx.NewClient(
		"test",
		"http://127.0.0.1:1",
		httpx.ClientOptions{Timeout: time.Second},
	)
	require.NoError(t, err)

	callErr := client.Call(t.Context(), "/v1/echo", echoRequest{}, &echoResponse{})
	require.Error(t, callErr)

	var domain *httpx.Error
	assert.False(
		t,
		errors.As(callErr, &domain),
		"an unreachable service must not look like an answer",
	)
}

func TestCallReportsACancelledContextAsAFailure(t *testing.T) {
	// The handler holds the request open until the test is done with it. Closing release first
	// lets the handler return, so shutting the server down cannot block on it.
	release := make(chan struct{})

	client := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-release:
		}
	})

	t.Cleanup(func() { close(release) })

	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()

	err := client.Call(ctx, "/v1/echo", echoRequest{}, &echoResponse{})
	require.Error(t, err)

	var domain *httpx.Error
	assert.False(t, errors.As(err, &domain))
}

func TestCallForwardsTheRequestID(t *testing.T) {
	var got string

	client := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get(reqid.Header)
		httpx.WriteJSON(w, echoResponse{})
	})

	ctx := reqid.NewContext(t.Context(), "01a0c5e1-0000-7000-8000-000000000000")
	require.NoError(t, client.Call(ctx, "/v1/echo", echoRequest{}, &echoResponse{}))

	assert.Equal(t, "01a0c5e1-0000-7000-8000-000000000000", got)
}

// A call that returns nothing useful still has to report failure, so void operations decode
// through the same path as everything else.
func TestCallAcceptsNoResponseTarget(t *testing.T) {
	client := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteJSON(w, struct{}{})
	})

	require.NoError(t, client.Call(t.Context(), "/v1/void", echoRequest{}, nil))
}

func TestWriteErrorProducesADecodableEnvelope(t *testing.T) {
	client := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteError(w, httpx.CodeConflict, "That username is already taken.")
	})

	err := client.Call(t.Context(), "/v1/echo", echoRequest{}, &echoResponse{})

	var domain *httpx.Error
	require.True(t, errors.As(err, &domain))
	assert.Equal(t, httpx.CodeConflict, domain.Code)
	assert.Equal(t, "That username is already taken.", domain.Message)
}
