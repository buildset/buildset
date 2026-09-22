package httpapi

import (
	"errors"

	"github.com/buildset/buildset/authz"
	"github.com/buildset/buildset/pkg/httpx"
)

// Classify is the one place authz's errors become a wire code and a sentence written for a
// visitor.
//
// Both of these are reachable only through a malformed request: a role name that is not in the
// roles table, or a reference that is not one. They used to reach the site untranslated and render
// as an internal error, which told a visitor the system had broken when in fact their request had.
// The in-process adapter uses this too, so both deployments answer the same way.
func Classify(err error) (httpx.Code, string, bool) {
	switch {
	case err == nil:
		return "", "", false
	case errors.Is(err, authz.ErrUnknownRole):
		return httpx.CodeInvalidInput, "That role does not exist.", true
	case errors.Is(err, authz.ErrInvalidAction):
		return httpx.CodeInvalidInput, err.Error(), true
	default:
		return "", "", false
	}
}
