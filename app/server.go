package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	authui "github.com/buildset/buildset/auth/ui"
	"github.com/buildset/buildset/web"
)

// Routes builds the root mux. Service handlers are mounted here and nowhere else, which keeps
// every service unaware that the others are served from the same process.
//
// The health probes and the middleware chain are not here: pkg/serve owns those, so this binary
// and the four split ones answer them identically.
func Routes(cfg *Config, svc *services, logger *slog.Logger) (http.Handler, error) {
	mux := http.NewServeMux()

	// Who may create an account is an authorization question, so it is answered here rather than
	// inside auth. Public sign-up is a configuration switch; otherwise it takes a permission.
	registrationPolicy := authui.RegistrationPolicyFunc(func(ctx context.Context, actorRef string) (bool, error) {
		if cfg.Auth.RegistrationOpen {
			return true, nil
		}

		if actorRef == "" {
			return false, nil
		}

		return svc.authz.Can(ctx, actorRef, web.ActionUserCreate, web.AnyUserResource)
	})

	authHandler, err := authui.NewHandler(svc.auth, registrationPolicy, authui.Config{
		SessionCookieName: cfg.SessionCookieName,
		SecureCookies:     cfg.SecureCookies,
	}, logger)
	if err != nil {
		return nil, fmt.Errorf("build auth handler: %w", err)
	}

	authHandler.Register(mux)

	site, err := web.New(web.Dependencies{
		Auth:    directAuth{service: svc.auth, pages: authHandler},
		Authz:   directAuthz{service: svc.authz},
		Content: directContent{service: svc.content},
	}, web.Config{
		SessionCookieName: cfg.SessionCookieName,
		SecureCookies:     cfg.SecureCookies,
		SiteTitle:         cfg.SiteTitle,
	}, logger)
	if err != nil {
		return nil, fmt.Errorf("build web server: %w", err)
	}

	// The site takes every path the routes above did not claim.
	mux.Handle("/", site.Handler())

	return mux, nil
}
