package app

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	authui "github.com/buildset/buildset/auth/ui"
	"github.com/buildset/buildset/web"
)

// NewHandler builds the root mux. Service handlers are mounted here and nowhere else, which keeps
// every service unaware that the others are served from the same process.
func NewHandler(cfg *Config, db *sql.DB, svc *services, logger *slog.Logger) (http.Handler, error) {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writePlain(w, http.StatusOK, "ok")
	})

	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		if err := db.PingContext(ctx); err != nil {
			writePlain(w, http.StatusServiceUnavailable, "database unavailable")

			return
		}

		writePlain(w, http.StatusOK, "ready")
	})

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

	// Rejects cross-origin state-changing requests using Sec-Fetch-Site, falling back to Origin.
	// TODO: requests carrying neither header are allowed through. Combined with SameSite=Lax that
	// leaves only pre-2023 browsers exposed; add session-bound form tokens if those must be supported.
	handler := http.NewCrossOriginProtection().Handler(mux)

	// Request tagging, logging, and panic recovery sit here rather than inside each service,
	// because they are a property of this process. A service split into its own binary adds them
	// in its own main. The identifier is applied first so everything below can log it.
	return withRequestID(recoverPanics(logger, logRequests(logger, handler))), nil
}

func writePlain(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	w.Write([]byte(body))
}
