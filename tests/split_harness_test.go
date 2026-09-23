package tests

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/buildset/buildset/app/authapp"
	"github.com/buildset/buildset/app/authzapp"
	"github.com/buildset/buildset/app/contentapp"
	"github.com/buildset/buildset/app/webapp"
	"github.com/buildset/buildset/pkg/config"
	"github.com/buildset/buildset/pkg/serve"
	"github.com/buildset/buildset/pkg/storage"
	"github.com/buildset/buildset/web/remote"
	"github.com/stretchr/testify/require"
)

// identityPaths are the paths the gateway sends to the identity service. They are the same list
// the Caddyfile carries, and they are exact matches rather than prefixes.
var identityPaths = []string{"/setup", "/login", "/logout", "/register", "/password"}

// newSplitHarness boots the four services behind a proxy carrying the gateway's routing, so the
// browser sees one origin as it does in compose. They share one SQLite file, which is not how they
// are deployed, but what a split breaks is the boundary between the processes.
func newSplitHarness(t *testing.T) *harness {
	t.Helper()

	browser := newBrowser(t)

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	database := storage.Config{
		Driver: storage.DriverSQLite,
		Path:   filepath.Join(t.TempDir(), "test.db"),
	}

	authzURL := startAuthz(t, logger, database)
	contentURL := startContent(t, logger, database)
	authURL := startAuth(t, logger, database, authzURL)
	webURL := startWeb(t, logger, authURL, authzURL, contentURL)

	gateway := httptest.NewServer(newGateway(t, authURL, webURL))
	t.Cleanup(gateway.Close)

	return &harness{url: gateway.URL, browser: browser}
}

// newGateway is the Caddyfile in twenty lines: the identity paths to one service, everything else
// to the site.
func newGateway(t *testing.T, authURL, webURL string) http.Handler {
	t.Helper()

	identity := newProxy(t, authURL)
	site := newProxy(t, webURL)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if slices.Contains(identityPaths, r.URL.Path) {
			identity.ServeHTTP(w, r)

			return
		}

		site.ServeHTTP(w, r)
	})
}

func newProxy(t *testing.T, target string) *httputil.ReverseProxy {
	t.Helper()

	parsed, err := url.Parse(target)
	require.NoError(t, err)

	return &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			r.SetURL(parsed)
			// The original Host is preserved, because both services set cookies for it.
			r.Out.Host = r.In.Host
		},
	}
}

func startAuthz(t *testing.T, logger *slog.Logger, database storage.Config) string {
	t.Helper()

	service, err := authzapp.New(context.Background(), &authzapp.Config{
		Port: "8080", ShutdownTimeout: time.Second,
		Database: database,
	}, logger)
	require.NoError(t, err)
	t.Cleanup(func() { _ = service.Close() })

	return startServer(
		t,
		serve.Options{Name: "authz", Logger: logger, Routes: service.Routes(), Ready: service.Ping},
	)
}

func startContent(t *testing.T, logger *slog.Logger, database storage.Config) string {
	t.Helper()

	service, err := contentapp.New(context.Background(), &contentapp.Config{
		Port: "8080", ShutdownTimeout: time.Second,
		Database: database,
	}, logger)
	require.NoError(t, err)
	t.Cleanup(func() { _ = service.Close() })

	return startServer(
		t,
		serve.Options{
			Name:   "content",
			Logger: logger,
			Routes: service.Routes(),
			Ready:  service.Ping,
		},
	)
}

func startAuth(t *testing.T, logger *slog.Logger, database storage.Config, authzURL string) string {
	t.Helper()

	service, err := authapp.New(context.Background(), &authapp.Config{
		Port: "8080", ShutdownTimeout: time.Second,
		// The test server speaks plain HTTP, so a Secure cookie would never come back.
		Cookie:   config.Cookie{Name: config.SessionCookieName, Secure: false},
		Database: database,
		// The lowest cost bcrypt accepts, because these tests sign in repeatedly.
		BcryptCost:       10,
		SessionTTL:       time.Hour,
		RegistrationOpen: true,
		AdminRole:        "admin",
		AuthzURL:         authzURL,
		HTTPTimeout:      5 * time.Second,
	}, logger)
	require.NoError(t, err)
	t.Cleanup(func() { _ = service.Close() })

	return startServer(t, serve.Options{
		Name:        "auth",
		Logger:      logger,
		Routes:      service.Routes(),
		Ready:       service.Ping,
		CrossOrigin: true,
	})
}

func startWeb(t *testing.T, logger *slog.Logger, authURL, authzURL, contentURL string) string {
	t.Helper()

	site, err := webapp.New(&webapp.Config{
		Port: "8080", ShutdownTimeout: time.Second,
		Cookie:      config.Cookie{Name: config.SessionCookieName, Secure: false},
		SiteTitle:   "Test Blog",
		AuthURL:     authURL,
		AuthzURL:    authzURL,
		ContentURL:  contentURL,
		HTTPTimeout: 5 * time.Second,
		URLs:        remote.DefaultURLs(),
	}, logger)
	require.NoError(t, err)

	return startServer(t, serve.Options{
		Name: "web", Logger: logger, Routes: site.Routes(), CrossOrigin: true,
	})
}

// startServer serves one binary's routes through the same assembly its own main would use, so the
// health probes and the middleware are the real ones.
func startServer(t *testing.T, options serve.Options) string {
	t.Helper()

	server := httptest.NewServer(serve.Handler(options))
	t.Cleanup(server.Close)

	return server.URL
}
