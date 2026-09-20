package ui

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/nasermirzaei89/ms/auth"
)

func (h *Handler) loginForm(w http.ResponseWriter, r *http.Request) {
	next := safeNext(r.URL.Query().Get("next"))

	if _, _, err := h.currentSession(r); err == nil {
		http.Redirect(w, r, next, http.StatusSeeOther)

		return
	}

	h.render(w, r, http.StatusOK, "login.gohtml", pageData{
		Title:            "Sign in",
		Next:             next,
		RegistrationOpen: h.anonymousMayRegister(r),
	})
}

func (h *Handler) loginSubmit(w http.ResponseWriter, r *http.Request) {
	if !h.parseForm(w, r) {
		return
	}

	var (
		username = r.PostFormValue("username")
		next     = safeNext(r.PostFormValue("next"))
	)

	user, err := h.service.Authenticate(r.Context(), username, r.PostFormValue("password"))
	if err != nil {
		if !errors.Is(err, auth.ErrInvalidCredentials) {
			h.renderInternalError(w, r, err, "authenticate user")

			return
		}

		// The same message for an unknown username and a wrong password, so the form cannot be
		// used to find out which accounts exist.
		h.render(w, r, http.StatusUnauthorized, "login.gohtml", pageData{
			Title:            "Sign in",
			ErrorMessage:     auth.ErrInvalidCredentials.Error(),
			Next:             next,
			Username:         username,
			RegistrationOpen: h.anonymousMayRegister(r),
		})

		return
	}

	if err := h.startSession(w, r, user); err != nil {
		h.renderInternalError(w, r, err, "start session after login")

		return
	}

	http.Redirect(w, r, next, http.StatusSeeOther)
}

// anonymousMayRegister decides whether the sign-in page offers a link to create an account. A
// policy that cannot answer hides the link rather than failing the page.
func (h *Handler) anonymousMayRegister(r *http.Request) bool {
	allowed, err := h.policy.MayRegister(r.Context(), "")
	if err != nil {
		h.logger.WarnContext(r.Context(), "check registration policy", slog.Any("error", err))

		return false
	}

	return allowed
}

// logoutSubmit is POST only. A logout reachable by GET can be triggered by any image tag on any
// other site.
func (h *Handler) logoutSubmit(w http.ResponseWriter, r *http.Request) {
	if token := h.sessionToken(r); token != "" {
		if err := h.service.RevokeSession(r.Context(), token); err != nil {
			h.renderInternalError(w, r, err, "revoke session")

			return
		}
	}

	h.clearSessionCookie(w)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
