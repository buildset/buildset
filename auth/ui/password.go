package ui

import (
	"errors"
	"net/http"

	"github.com/buildset/buildset/auth"
)

func (h *Handler) passwordForm(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := h.requireSession(w, r); !ok {
		return
	}

	data := pageData{Title: "Change your password"}
	if r.URL.Query().Has("changed") {
		data.Notice = "Your password has been changed."
	}

	h.render(w, r, http.StatusOK, "password.gohtml", data)
}

func (h *Handler) passwordSubmit(w http.ResponseWriter, r *http.Request) {
	session, user, ok := h.requireSession(w, r)
	if !ok {
		return
	}

	if !h.parseForm(w, r) {
		return
	}

	req := auth.ChangePasswordRequest{
		UserID:          user.ID,
		CurrentPassword: r.PostFormValue("current_password"),
		NewPassword:     r.PostFormValue("new_password"),
		KeepSessionID:   session.ID,
	}

	if err := h.service.ChangePassword(r.Context(), req); err != nil {
		if !isValidationError(err) && !errors.Is(err, auth.ErrInvalidCredentials) {
			h.renderInternalError(w, r, err, "change password")

			return
		}

		message := userFacingError(err, "That password could not be changed.")
		if errors.Is(err, auth.ErrInvalidCredentials) {
			message = "Your current password is not correct."
		}

		h.render(w, r, http.StatusBadRequest, "password.gohtml", pageData{
			Title:        "Change your password",
			ErrorMessage: message,
		})

		return
	}

	http.Redirect(w, r, "/password?changed", http.StatusSeeOther)
}

// requireSession sends an unauthenticated visitor to the login page and reports whether the caller
// should stop.
func (h *Handler) requireSession(
	w http.ResponseWriter,
	r *http.Request,
) (*auth.Session, *auth.User, bool) {
	session, user, err := h.currentSession(r)
	if err == nil {
		return session, user, true
	}

	if !errors.Is(err, auth.ErrSessionNotFound) {
		h.renderInternalError(w, r, err, "resolve session")

		return nil, nil, false
	}

	h.clearSessionCookie(w)

	if r.Method != http.MethodGet {
		// Redirecting a POST would throw away what the visitor typed, so say so instead.
		h.renderError(
			w,
			r,
			http.StatusUnauthorized,
			"Your session has expired. Sign in again and retry.",
		)

		return nil, nil, false
	}

	http.Redirect(w, r, h.LoginURL(r.URL.RequestURI()), http.StatusSeeOther)

	return nil, nil, false
}
