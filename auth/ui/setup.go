package ui

import (
	"errors"
	"net/http"

	"github.com/buildset/buildset/auth"
)

// setupGate closes the first-run pages for good once a user exists. It answers 404 rather than 403
// so a running instance does not advertise that a setup route was ever there.
func (h *Handler) setupGate(w http.ResponseWriter, r *http.Request) bool {
	open, err := h.service.SetupOpen(r.Context())
	if err != nil {
		h.renderInternalError(w, r, err, "check setup state")

		return false
	}

	if !open {
		h.renderError(w, r, http.StatusNotFound, "There is nothing here.")

		return false
	}

	return true
}

func (h *Handler) setupForm(w http.ResponseWriter, r *http.Request) {
	if !h.setupGate(w, r) {
		return
	}

	h.render(w, r, http.StatusOK, "setup.gohtml", pageData{Title: "Set up this blog"})
}

func (h *Handler) setupSubmit(w http.ResponseWriter, r *http.Request) {
	if !h.parseForm(w, r) || !h.setupGate(w, r) {
		return
	}

	req := auth.RegisterRequest{
		Username: r.PostFormValue("username"),
		Name:     r.PostFormValue("name"),
		Password: r.PostFormValue("password"),
	}

	user, err := h.service.CompleteSetup(r.Context(), req)
	if err != nil {
		if errors.Is(err, auth.ErrSetupClosed) {
			h.renderError(w, r, http.StatusNotFound, "There is nothing here.")

			return
		}

		if !isValidationError(err) {
			h.renderInternalError(w, r, err, "complete setup")

			return
		}

		// The submitted password is never echoed back into the form.
		h.render(w, r, http.StatusBadRequest, "setup.gohtml", pageData{
			Title:        "Set up this blog",
			ErrorMessage: userFacingError(err, "That account could not be created."),
			Username:     req.Username,
			Name:         req.Name,
		})

		return
	}

	if err := h.startSession(w, r, user); err != nil {
		h.renderInternalError(w, r, err, "start session after setup")

		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}
