package httpapi

import (
	"errors"

	"github.com/buildset/buildset/content"
	"github.com/buildset/buildset/pkg/httpx"
)

// Classify is the one place content's errors become a wire code and a sentence for a visitor. The
// in-process adapter uses it too, so both topologies answer identically. A false result means the
// failure is the system's, not the request's.
func Classify(err error) (httpx.Code, string, bool) {
	switch {
	case err == nil:
		return "", "", false
	case errors.Is(err, content.ErrPostNotFound):
		return httpx.CodeNotFound, "That post could not be found.", true
	case errors.Is(err, content.ErrInvalidPost),
		errors.Is(err, content.ErrInvalidStatus),
		errors.Is(err, content.ErrInvalidTransition),
		errors.Is(err, content.ErrUnsupportedContent):
		return httpx.CodeInvalidInput, err.Error(), true
	default:
		return "", "", false
	}
}
