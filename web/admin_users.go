package web

import (
	"errors"
	"net/http"
	"slices"
)

type adminUsersContent struct {
	Users []User
	// CanCreate controls a link to the identity service's form. This site never renders a password
	// field of its own.
	CanCreate   bool
	RegisterURL string
}

type adminUserContent struct {
	User      User
	AllRoles  []string
	HeldRoles []string
	CanAssign bool
	CanDelete bool
	IsSelf    bool
}

func (s *Server) adminUsers(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requirePermission(w, r, ActionUserRead, anyUserResource); !ok {
		return
	}

	users, err := s.deps.Auth.ListUsers(r.Context(), 0)
	if err != nil {
		s.renderInternalError(w, r, err, "list users")

		return
	}

	viewer, _ := userFromContext(r.Context())

	canCreate, err := s.can(r.Context(), viewer, ActionUserCreate, anyUserResource)
	if err != nil {
		s.renderInternalError(w, r, err, "check user permission")

		return
	}

	data := s.newLayoutData(r, "Users")
	if r.URL.Query().Has("deleted") {
		data.Notice = "The account has been deleted."
	}

	data.Content = adminUsersContent{
		Users:       users,
		CanCreate:   canCreate,
		RegisterURL: s.deps.Auth.RegisterURL(),
	}

	s.render(w, r, http.StatusOK, "admin_users.gohtml", data)
}

func (s *Server) adminUser(w http.ResponseWriter, r *http.Request) {
	viewer, subject, ok := s.authorizeUser(w, r, ActionUserRead)
	if !ok {
		return
	}

	content, err := s.buildUserContent(r, viewer, subject)
	if err != nil {
		s.renderInternalError(w, r, err, "build user page")

		return
	}

	data := s.newLayoutData(r, subject.Username)
	if r.URL.Query().Has("saved") {
		data.Notice = "Roles have been updated."
	}

	data.Content = content

	s.render(w, r, http.StatusOK, "admin_user.gohtml", data)
}

// setUserRoles replaces the whole role set from the submitted checkboxes, so resubmitting the same
// form changes nothing rather than failing or duplicating.
func (s *Server) setUserRoles(w http.ResponseWriter, r *http.Request) {
	viewer, subject, ok := s.authorizeUser(w, r, ActionRoleAssign)
	if !ok {
		return
	}

	if !s.parseForm(w, r) {
		return
	}

	available, err := s.deps.Authz.ListRoles(r.Context())
	if err != nil {
		s.renderInternalError(w, r, err, "list roles")

		return
	}

	held, err := s.deps.Authz.SubjectRoles(r.Context(), subject.Ref)
	if err != nil {
		s.renderInternalError(w, r, err, "list subject roles")

		return
	}

	// Only roles that exist are considered, so a hand-built form cannot invent one.
	requested := make([]string, 0, len(available))

	for _, role := range r.PostForm["roles"] {
		if slices.Contains(available, role) {
			requested = append(requested, role)
		}
	}

	// Removing your own last administrator role would lock the site's only administrator out, and
	// nothing else could grant it back.
	if subject.Ref == viewer.Ref && !slices.Contains(requested, administratorRole) && slices.Contains(held, administratorRole) {
		content, buildErr := s.buildUserContent(r, viewer, subject)
		if buildErr != nil {
			s.renderInternalError(w, r, buildErr, "build user page")

			return
		}

		data := s.newLayoutData(r, subject.Username)
		data.ErrorMessage = "You cannot remove your own administrator role."
		data.Content = content

		s.render(w, r, http.StatusBadRequest, "admin_user.gohtml", data)

		return
	}

	for _, role := range available {
		var err error

		switch {
		case slices.Contains(requested, role) && !slices.Contains(held, role):
			err = s.deps.Authz.AssignRole(r.Context(), subject.Ref, role)
		case !slices.Contains(requested, role) && slices.Contains(held, role):
			err = s.deps.Authz.RevokeRole(r.Context(), subject.Ref, role)
		}

		if err != nil {
			s.renderInternalError(w, r, err, "change role")

			return
		}
	}

	http.Redirect(w, r, "/admin/users/"+subject.ID+"?saved", http.StatusSeeOther)
}

// authorizeUser loads the user named in the path and checks one permission over them.
func (s *Server) authorizeUser(w http.ResponseWriter, r *http.Request, action string) (*User, *User, bool) {
	id := r.PathValue("id")

	resource, err := userResource(id)
	if err != nil {
		s.renderError(w, r, http.StatusNotFound, "There is no such user.")

		return nil, nil, false
	}

	viewer, ok := s.requirePermission(w, r, action, resource)
	if !ok {
		return nil, nil, false
	}

	subject, err := s.deps.Auth.GetUserByRef(r.Context(), resource)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			s.renderError(w, r, http.StatusNotFound, "There is no such user.")

			return nil, nil, false
		}

		s.renderInternalError(w, r, err, "get user")

		return nil, nil, false
	}

	return viewer, subject, true
}

func (s *Server) buildUserContent(r *http.Request, viewer, subject *User) (adminUserContent, error) {
	available, err := s.deps.Authz.ListRoles(r.Context())
	if err != nil {
		return adminUserContent{}, err
	}

	held, err := s.deps.Authz.SubjectRoles(r.Context(), subject.Ref)
	if err != nil {
		return adminUserContent{}, err
	}

	canAssign, err := s.can(r.Context(), viewer, ActionRoleAssign, subject.Ref)
	if err != nil {
		return adminUserContent{}, err
	}

	canDelete, err := s.can(r.Context(), viewer, ActionUserDelete, subject.Ref)
	if err != nil {
		return adminUserContent{}, err
	}

	isSelf := subject.Ref == viewer.Ref

	return adminUserContent{
		User:      *subject,
		AllRoles:  available,
		HeldRoles: held,
		CanAssign: canAssign,
		// Deleting your own account here would sign you out mid-administration and could remove
		// the last administrator. Account closure belongs on a page of its own.
		CanDelete: canDelete && !isSelf,
		IsSelf:    isSelf,
	}, nil
}

func (s *Server) deleteUser(w http.ResponseWriter, r *http.Request) {
	viewer, subject, ok := s.authorizeUser(w, r, ActionUserDelete)
	if !ok {
		return
	}

	if subject.Ref == viewer.Ref {
		s.renderError(w, r, http.StatusBadRequest, "You cannot delete your own account here.")

		return
	}

	// The roles and grants go first, and the account second, for the same reason as deleting a
	// post: these are two services and cannot share a transaction, and this is the order where a
	// failure leaves something an administrator can retry rather than something left behind with
	// nothing to name it. An account stripped of its roles is an account, and deleting it again
	// finishes the job.
	//
	// Roles and grants keyed to a reference that no longer resolves would apply to whoever got
	// that reference next, so they go with the account.
	if err := s.deps.Authz.PurgeSubject(r.Context(), subject.Ref); err != nil {
		s.renderInternalError(w, r, err, "purge user grants")

		return
	}

	if err := s.deps.Auth.DeleteUser(r.Context(), subject.Ref); err != nil {
		s.renderInternalError(w, r, err, "delete user")

		return
	}

	http.Redirect(w, r, "/admin/users?deleted", http.StatusSeeOther)
}
