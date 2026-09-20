package web

import "net/http"

type adminContent struct {
	CanManagePosts bool
	CanManageUsers bool
}

// adminIndex is reachable by anyone who can do something on one of the pages it links to, so an
// author sees it without being an administrator.
func (s *Server) adminIndex(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}

	canManagePosts, err := s.can(r.Context(), user, ActionPostCreate, anyPostResource)
	if err != nil {
		s.renderInternalError(w, r, err, "check post permission")

		return
	}

	canManageUsers, err := s.can(r.Context(), user, ActionUserRead, anyUserResource)
	if err != nil {
		s.renderInternalError(w, r, err, "check user permission")

		return
	}

	if !canManagePosts && !canManageUsers {
		s.logDenied(r, ActionPostCreate, anyPostResource)
		s.renderError(w, r, http.StatusForbidden, "You do not have access to that.")

		return
	}

	data := s.newLayoutData(r, "Administration")
	data.Content = adminContent{CanManagePosts: canManagePosts, CanManageUsers: canManageUsers}

	s.render(w, r, http.StatusOK, "admin.gohtml", data)
}
