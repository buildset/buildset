package ui

import (
	"net/http"
	"time"
)

// setSessionCookie is the only place session cookie attributes are decided.
//
// TODO: the __Host- prefix would force Secure, Path=/, and no Domain at the browser. It also makes
// plain-HTTP local development impossible, so it waits until dev runs over TLS (RFC 6265bis 4.1.3).
func (h *Handler) setSessionCookie(w http.ResponseWriter, token string, expiresAt time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:  h.config.SessionCookieName,
		Value: token,
		Path:  "/",
		// No Domain, so the cookie is host-only and cannot reach a sibling subdomain.
		Expires:  expiresAt,
		MaxAge:   int(time.Until(expiresAt).Seconds()),
		HttpOnly: true,
		Secure:   h.config.SecureCookies,
		// Lax still blocks cross-site POST, and unlike Strict it survives following a link into
		// the site from somewhere else.
		SameSite: http.SameSiteLaxMode,
	})
}

func (h *Handler) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     h.config.SessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   h.config.SecureCookies,
		SameSite: http.SameSiteLaxMode,
	})
}

func (h *Handler) sessionToken(r *http.Request) string {
	cookie, err := r.Cookie(h.config.SessionCookieName)
	if err != nil {
		return ""
	}

	return cookie.Value
}
