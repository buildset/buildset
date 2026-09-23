package httpapi

import (
	"errors"

	"github.com/buildset/buildset/authz"
	"github.com/buildset/buildset/pkg/httpx"
)

// Classify is the one place authz's errors become a wire code and a sentence for a visitor. Both
// are reachable only through a malformed request, and untranslated they would render as an internal
// error. The in-process adapter uses this too, so both topologies answer the same way.
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
