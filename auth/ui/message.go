package ui

import (
	"errors"

	"github.com/nasermirzaei89/ms/auth"
	"github.com/nasermirzaei89/ms/auth/hash"
)

// userFacingError decides which failures are safe and useful to show on a form. Anything not
// listed here becomes the fallback, so an internal message never reaches the browser.
func userFacingError(err error, fallback string) string {
	switch {
	case errors.Is(err, auth.ErrUsernameTaken):
		return "That username is already taken."
	case errors.Is(err, auth.ErrInvalidUsername),
		errors.Is(err, hash.ErrPasswordTooShort),
		errors.Is(err, hash.ErrPasswordTooLong):
		return err.Error()
	case errors.Is(err, auth.ErrInvalidCredentials):
		return auth.ErrInvalidCredentials.Error()
	default:
		return fallback
	}
}

// isValidationError separates what the visitor can fix by retyping the form from what they cannot.
// Everything else is the system's fault and must not be rendered as a form error.
func isValidationError(err error) bool {
	return errors.Is(err, auth.ErrUsernameTaken) ||
		errors.Is(err, auth.ErrInvalidUsername) ||
		errors.Is(err, hash.ErrPasswordTooShort) ||
		errors.Is(err, hash.ErrPasswordTooLong)
}
