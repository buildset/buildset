// Package webapp is the composition root of the site running on its own. It has no database and
// asks other services for everything through web/remote, so it links no service code, no password
// hashing and no SQL driver.
package webapp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	authclient "github.com/buildset/buildset/auth/client"
	authzclient "github.com/buildset/buildset/authz/client"
	contentclient "github.com/buildset/buildset/content/client"
	"github.com/buildset/buildset/pkg/config"
	"github.com/buildset/buildset/pkg/httpx"
	"github.com/buildset/buildset/pkg/serve"
	"github.com/buildset/buildset/web"
	"github.com/buildset/buildset/web/remote"
	"github.com/nasermirzaei89/env"
)

var errInvalidConfig = errors.New("invalid configuration")

type Config struct {
	config.Server

	Log    config.Log
	Cookie config.Cookie

	SiteTitle string

	AuthURL     string
	AuthzURL    string
	ContentURL  string
	HTTPTimeout time.Duration

	// The identity service's pages, relative because the gateway puts both services on one origin.
	URLs remote.URLs
}

func LoadConfig() (*Config, error) {
	log, err := config.LoadLog()
	if err != nil {
		return nil, err
	}

	urls := map[string]*string{}

	cfg := &Config{
		Server:      config.LoadServer(),
		Log:         log,
		Cookie:      config.LoadCookie(),
		SiteTitle:   env.GetString("SITE_TITLE", "Blog"),
		HTTPTimeout: env.GetDuration("HTTP_TIMEOUT", 5*time.Second),
		URLs:        loadURLs(),
	}

	urls["AUTH_URL"] = &cfg.AuthURL
	urls["AUTHZ_URL"] = &cfg.AuthzURL
	urls["CONTENT_URL"] = &cfg.ContentURL

	for key, target := range urls {
		value, err := config.LoadURL(key)
		if err != nil {
			return nil, err
		}

		*target = value
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Keeping these configurable makes moving a page a setting rather than a release of two binaries.
func loadURLs() remote.URLs {
	defaults := remote.DefaultURLs()

	return remote.URLs{
		Login:    env.GetString("AUTH_LOGIN_PATH", defaults.Login),
		Logout:   env.GetString("AUTH_LOGOUT_PATH", defaults.Logout),
		Password: env.GetString("AUTH_PASSWORD_PATH", defaults.Password),
		Register: env.GetString("AUTH_REGISTER_PATH", defaults.Register),
	}
}

func (c *Config) Validate() error {
	if err := c.Server.Validate(); err != nil {
		return err
	}

	if c.HTTPTimeout <= 0 {
		return fmt.Errorf("%w: HTTP_TIMEOUT must be positive, got %s", errInvalidConfig, c.HTTPTimeout)
	}

	return nil
}

type Site struct {
	routes http.Handler
}

func New(cfg *Config, logger *slog.Logger) (*Site, error) {
	options := httpx.ClientOptions{Timeout: cfg.HTTPTimeout}

	authRemote, err := authclient.New(cfg.AuthURL, options)
	if err != nil {
		return nil, fmt.Errorf("build auth client: %w", err)
	}

	authzRemote, err := authzclient.New(cfg.AuthzURL, options)
	if err != nil {
		return nil, fmt.Errorf("build authz client: %w", err)
	}

	contentRemote, err := contentclient.New(cfg.ContentURL, options)
	if err != nil {
		return nil, fmt.Errorf("build content client: %w", err)
	}

	site, err := web.New(web.Dependencies{
		Auth:    remote.NewAuth(authRemote, cfg.URLs),
		Authz:   remote.NewAuthz(authzRemote),
		Content: remote.NewContent(contentRemote),
	}, web.Config{
		SessionCookieName: cfg.Cookie.Name,
		SecureCookies:     cfg.Cookie.Secure,
		SiteTitle:         cfg.SiteTitle,
	}, logger)
	if err != nil {
		return nil, fmt.Errorf("build web server: %w", err)
	}

	return &Site{routes: site.Handler()}, nil
}

func (s *Site) Routes() http.Handler { return s.routes }

func Run(ctx context.Context) error {
	cfg, err := LoadConfig()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	logger := serve.NewLogger(cfg.Log)
	slog.SetDefault(logger)

	site, err := New(cfg, logger)
	if err != nil {
		return err
	}

	return serve.Run(ctx, serve.Options{
		Name:            "web",
		Address:         cfg.Address(),
		ShutdownTimeout: cfg.ShutdownTimeout,
		Logger:          logger,
		Routes:          site.Routes(),
		// Ready is deliberately nil. Following a dependency's readiness would make restarting the
		// identity service leave the gateway nothing to route to. A dependency being down is
		// answered per request.
		Ready: nil,
		// A browser posts forms here, so cross-origin protection applies and an inbound request
		// identifier must not be trusted.
		CrossOrigin:    true,
		TrustRequestID: false,
	})
}
