package httpapi

import (
	"errors"

	"github.com/buildset/buildset/auth"
	"github.com/buildset/buildset/auth/hash"
	"github.com/buildset/buildset/pkg/httpx"
)

// Classify is the one place auth's errors become a wire code and a sentence written for a visitor.
//
// The in-process adapter uses it too, so the single binary and the split answer identically. A
// false result means the failure is the system's, not the request's, and the caller must not turn
// it into an answer.
func Classify(err error) (httpx.Code, string, bool) {
	switch {
	case err == nil:
		return "", "", false
	case errors.Is(err, auth.ErrSessionNotFound), errors.Is(err, auth.ErrUserNotFound):
		return httpx.CodeNotFound, "That account could not be found.", true
	case errors.Is(err, auth.ErrUsernameTaken):
		return httpx.CodeConflict, "That username is already taken.", true
	case errors.Is(err, auth.ErrInvalidUsername),
		errors.Is(err, hash.ErrPasswordTooShort),
		errors.Is(err, hash.ErrPasswordTooLong):
		return httpx.CodeInvalidInput, err.Error(), true
	default:
		return "", "", false
	}
}
