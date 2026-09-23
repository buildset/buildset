package web

import (
	"context"
	"log/slog"
	"net/http"
)

// can asks the authorization service. A failure to reach it is never treated as permission.
func (s *Server) can(ctx context.Context, user *User, action, resource string) (bool, error) {
	if user == nil {
		return false, nil
	}

	return s.deps.Authz.Can(ctx, user.Ref, action, resource)
}

// requirePermission reports whether to continue. A denial on an administration page is a 403; on a
// public URL the caller answers 404, so the existence of the resource stays hidden.
func (s *Server) requirePermission(w http.ResponseWriter, r *http.Request, action, resource string) (*User, bool) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return nil, false
	}

	allowed, err := s.can(r.Context(), user, action, resource)
	if err != nil {
		s.renderInternalError(w, r, err, "check permission")

		return nil, false
	}

	if !allowed {
		s.renderError(w, r, http.StatusForbidden, "You do not have access to that.")

		return nil, false
	}

	return user, true
}

// logDenied records a refusal that is reported to the visitor as something else, so the real
// reason is not lost.
func (s *Server) logDenied(r *http.Request, action, resource string) {
	s.logger.InfoContext(r.Context(), "permission denied",
		slog.String("action", action),
		slog.String("resource", resource),
		slog.String("path", r.URL.Path),
	)
}
