package web

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
)

type contextKey struct{}

var userContextKey contextKey

// withSession resolves the session cookie once per request and puts the result in the context.
func (s *Server) withSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(s.config.SessionCookieName)
		if err != nil {
			next.ServeHTTP(w, r)

			return
		}

		user, err := s.deps.Auth.ResolveSession(r.Context(), cookie.Value)
		if err != nil {
			if !errors.Is(err, ErrNotFound) {
				// A failure to reach the identity service must never be read as "anonymous", so the
				// request stops here.
				s.logger.ErrorContext(r.Context(), "resolve session", slog.Any("error", err))
				s.renderError(w, r, http.StatusServiceUnavailable, "Sign-in is unavailable right now.")

				return
			}

			// The cookie points at nothing. Clearing it stops the browser presenting it forever.
			s.clearSessionCookie(w)
			next.ServeHTTP(w, r)

			return
		}

		// Pages rendered for a signed-in visitor must not be kept by any cache.
		w.Header().Set("Cache-Control", "no-store")

		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userContextKey, user)))
	})
}

// userFromContext returns the signed-in visitor, if there is one.
func userFromContext(ctx context.Context) (*User, bool) {
	user, ok := ctx.Value(userContextKey).(*User)

	return user, ok
}

// requireUser stops an unauthenticated request and reports whether the caller should continue.
func (s *Server) requireUser(w http.ResponseWriter, r *http.Request) (*User, bool) {
	user, ok := userFromContext(r.Context())
	if ok {
		return user, true
	}

	if r.Method != http.MethodGet {
		// A redirect would discard the submitted form, so tell the visitor instead of losing it.
		s.renderError(w, r, http.StatusUnauthorized, "Your session has expired. Sign in again and retry.")

		return nil, false
	}

	http.Redirect(w, r, s.deps.Auth.LoginURL(r.URL.RequestURI()), http.StatusSeeOther)

	return nil, false
}

// The attributes must mirror the ones auth sets, or the browser sees a different cookie.
func (s *Server) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     s.config.SessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.config.SecureCookies,
		SameSite: http.SameSiteLaxMode,
	})
}
