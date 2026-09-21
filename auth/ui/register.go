package ui

import (
	"errors"
	"net/http"

	"github.com/buildset/buildset/auth"
)

// registrationGate answers 404 when this visitor may not create an account, so a closed instance
// does not advertise the route to strangers.
//
// It returns the signed-in visitor, if there is one. An administrator reaching this page is adding
// somebody else's account, which is a different outcome from a stranger signing themselves up.
func (h *Handler) registrationGate(w http.ResponseWriter, r *http.Request) (*auth.User, bool) {
	actor, err := h.currentUser(r)
	if err != nil {
		h.renderInternalError(w, r, err, "resolve session")

		return nil, false
	}

	var actorRef string
	if actor != nil {
		actorRef = actor.Ref()
	}

	allowed, err := h.policy.MayRegister(r.Context(), actorRef)
	if err != nil {
		h.renderInternalError(w, r, err, "check registration policy")

		return nil, false
	}

	if !allowed {
		h.renderError(w, r, http.StatusNotFound, "There is nothing here.")

		return nil, false
	}

	return actor, true
}

func (h *Handler) registerForm(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.registrationGate(w, r)
	if !ok {
		return
	}

	h.render(w, r, http.StatusOK, "register.gohtml", pageData{
		Title:          registerTitle(actor),
		ForSomeoneElse: actor != nil,
	})
}

// registerTitle distinguishes signing yourself up from adding an account for somebody else.
func registerTitle(actor *auth.User) string {
	if actor != nil {
		return "Add a user"
	}

	return "Create an account"
}

func (h *Handler) registerSubmit(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.registrationGate(w, r)
	if !ok {
		return
	}

	if !h.parseForm(w, r) {
		return
	}

	req := auth.RegisterRequest{
		Username: r.PostFormValue("username"),
		Name:     r.PostFormValue("name"),
		Password: r.PostFormValue("password"),
	}

	user, err := h.service.Register(r.Context(), req)
	if err != nil {
		if !isValidationError(err) {
			h.renderInternalError(w, r, err, "register user")

			return
		}

		status := http.StatusBadRequest
		if errors.Is(err, auth.ErrUsernameTaken) {
			status = http.StatusConflict
		}

		h.render(w, r, status, "register.gohtml", pageData{
			Title:          registerTitle(actor),
			ErrorMessage:   userFacingError(err, "That account could not be created."),
			Username:       req.Username,
			Name:           req.Name,
			ForSomeoneElse: actor != nil,
		})

		return
	}

	// Somebody adding an account for another person must stay signed in as themselves.
	if actor != nil {
		http.Redirect(w, r, "/admin/users/"+user.ID, http.StatusSeeOther)

		return
	}

	if err := h.startSession(w, r, user); err != nil {
		h.renderInternalError(w, r, err, "start session after registration")

		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}
