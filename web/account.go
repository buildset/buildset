package web

import (
	"errors"
	"net/http"
)

type accountContent struct {
	User        User
	PasswordURL string
}

func (s *Server) accountForm(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}

	data := s.newLayoutData(r, "Your account")
	if r.URL.Query().Has("saved") {
		data.Notice = "Your account has been updated."
	}

	data.Content = accountContent{User: *user, PasswordURL: s.deps.Auth.PasswordURL()}

	s.render(w, r, http.StatusOK, "account.gohtml", data)
}

// accountSubmit edits the signed-in visitor's own profile, and only theirs. The reference comes
// from the session rather than from the form, so there is no identifier to tamper with.
func (s *Server) accountSubmit(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}

	if !s.parseForm(w, r) {
		return
	}

	var (
		username = r.PostFormValue("username")
		name     = r.PostFormValue("name")
	)

	_, err := s.deps.Auth.UpdateProfile(r.Context(), user.Ref, username, name)
	if err != nil {
		if !errors.Is(err, ErrInvalidInput) && !errors.Is(err, ErrConflict) {
			s.renderInternalError(w, r, err, "update profile")

			return
		}

		status := http.StatusBadRequest
		if errors.Is(err, ErrConflict) {
			status = http.StatusConflict
		}

		data := s.newLayoutData(r, "Your account")
		data.ErrorMessage = err.Error()
		data.Content = accountContent{
			User:        User{Ref: user.Ref, Username: username, Name: name},
			PasswordURL: s.deps.Auth.PasswordURL(),
		}

		s.render(w, r, status, "account.gohtml", data)

		return
	}

	http.Redirect(w, r, "/account?saved", http.StatusSeeOther)
}
