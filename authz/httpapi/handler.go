// Package httpapi serves the authorization service over HTTP. Subjects, actions and resources are
// opaque strings on the wire exactly as they are in the service.
package httpapi

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/buildset/buildset/authz"
	"github.com/buildset/buildset/pkg/api/authzapi"
	"github.com/buildset/buildset/pkg/httpx"
)

var errMissingDependency = errors.New("missing dependency")

type Handler struct {
	service *authz.Service
	logger  *slog.Logger
}

func NewHandler(service *authz.Service, logger *slog.Logger) (*Handler, error) {
	if service == nil {
		return nil, fmt.Errorf("%w: authz service must not be nil", errMissingDependency)
	}

	if logger == nil {
		return nil, fmt.Errorf("%w: logger must not be nil", errMissingDependency)
	}

	return &Handler{service: service, logger: logger}, nil
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST "+authzapi.PathCan, h.can)
	mux.HandleFunc("POST "+authzapi.PathGrant, h.grant)
	mux.HandleFunc("POST "+authzapi.PathAssignRole, h.assignRole)
	mux.HandleFunc("POST "+authzapi.PathRevokeRole, h.revokeRole)
	mux.HandleFunc("POST "+authzapi.PathSubjectRoles, h.subjectRoles)
	mux.HandleFunc("POST "+authzapi.PathListRoles, h.listRoles)
	mux.HandleFunc("POST "+authzapi.PathPurgeResource, h.purgeResource)
	mux.HandleFunc("POST "+authzapi.PathPurgeSubject, h.purgeSubject)
}

func (h *Handler) can(w http.ResponseWriter, r *http.Request) {
	var request authzapi.CanRequest
	if !httpx.DecodeJSON(w, r, &request) {
		return
	}

	allowed, err := h.service.Can(r.Context(), request.Subject, request.Action, request.Resource)
	if err != nil {
		h.fail(w, r, "can", err)

		return
	}

	httpx.WriteJSON(w, authzapi.CanResponse{Allowed: allowed})
}

func (h *Handler) grant(w http.ResponseWriter, r *http.Request) {
	var request authzapi.GrantRequest
	if !httpx.DecodeJSON(w, r, &request) {
		return
	}

	h.void(w, r, "grant", h.service.Grant(r.Context(), request.Subject, request.Actions, request.Resource))
}

func (h *Handler) assignRole(w http.ResponseWriter, r *http.Request) {
	var request authzapi.RoleRequest
	if !httpx.DecodeJSON(w, r, &request) {
		return
	}

	h.void(w, r, "assign role", h.service.AssignRole(r.Context(), request.Subject, request.Role))
}

func (h *Handler) revokeRole(w http.ResponseWriter, r *http.Request) {
	var request authzapi.RoleRequest
	if !httpx.DecodeJSON(w, r, &request) {
		return
	}

	h.void(w, r, "revoke role", h.service.RevokeRole(r.Context(), request.Subject, request.Role))
}

func (h *Handler) subjectRoles(w http.ResponseWriter, r *http.Request) {
	var request authzapi.SubjectRequest
	if !httpx.DecodeJSON(w, r, &request) {
		return
	}

	roles, err := h.service.SubjectRoles(r.Context(), request.Subject)
	if err != nil {
		h.fail(w, r, "subject roles", err)

		return
	}

	httpx.WriteJSON(w, authzapi.RolesResponse{Roles: roles})
}

func (h *Handler) listRoles(w http.ResponseWriter, r *http.Request) {
	var request authzapi.Empty
	if !httpx.DecodeJSON(w, r, &request) {
		return
	}

	roles, err := h.service.ListRoles(r.Context())
	if err != nil {
		h.fail(w, r, "list roles", err)

		return
	}

	// The description has no consumer, so only the names cross the wire.
	names := make([]string, 0, len(roles))
	for _, role := range roles {
		names = append(names, role.Name)
	}

	httpx.WriteJSON(w, authzapi.RolesResponse{Roles: names})
}

func (h *Handler) purgeResource(w http.ResponseWriter, r *http.Request) {
	var request authzapi.ResourceRequest
	if !httpx.DecodeJSON(w, r, &request) {
		return
	}

	h.void(w, r, "purge resource", h.service.PurgeResource(r.Context(), request.Resource))
}

func (h *Handler) purgeSubject(w http.ResponseWriter, r *http.Request) {
	var request authzapi.SubjectRequest
	if !httpx.DecodeJSON(w, r, &request) {
		return
	}

	h.void(w, r, "purge subject", h.service.PurgeSubject(r.Context(), request.Subject))
}

func (h *Handler) void(w http.ResponseWriter, r *http.Request, operation string, err error) {
	if err != nil {
		h.fail(w, r, operation, err)

		return
	}

	httpx.WriteJSON(w, authzapi.Empty{})
}

// fail turns a service error into an envelope the caller can act on, or into a failure it must not
// mistake for an answer.
func (h *Handler) fail(w http.ResponseWriter, r *http.Request, operation string, err error) {
	if code, message, ok := Classify(err); ok {
		httpx.WriteError(w, code, message)

		return
	}

	httpx.WriteInternal(r.Context(), w, h.logger, operation, err)
}
