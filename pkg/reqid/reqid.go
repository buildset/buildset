// Package reqid carries a request identifier through a context and across a service boundary. It
// sits below both the HTTP middleware that creates one and the client that forwards it, so neither
// has to import the other.
package reqid

import "context"

// Header carries the identifier between services and back to the client, so a report of a failure
// can be tied to the lines every process logged about it.
const Header = "X-Request-Id"

type contextKey struct{}

// NewContext returns a context carrying id.
func NewContext(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, contextKey{}, id)
}

// FromContext returns the identifier of the request being handled, if there is one.
func FromContext(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(contextKey{}).(string)

	return id, ok
}
