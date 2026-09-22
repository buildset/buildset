// Package httpapi serves the part of the identity service that other services need.
//
// Registering, authenticating, creating or revoking a session, changing a password and completing
// setup are deliberately absent. They are used only by the identity service's own pages, which run
// in the same process, so nothing reachable over the network can create a session or change a
// password.
package httpapi

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/buildset/buildset/auth"
	"github.com/buildset/buildset/pkg/api/authapi"
	"github.com/buildset/buildset/pkg/httpx"
	"github.com/buildset/buildset/pkg/ref"
)

type Handler struct {
	service *auth.Service
	logger  *slog.Logger
}

func NewHandler(service *auth.Service, logger *slog.Logger) (*Handler, error) {
	if service == nil {
		return nil, fmt.Errorf("auth service must not be nil")
	}

	if logger == nil {
		return nil, fmt.Errorf("logger must not be nil")
	}

	return &Handler{service: service, logger: logger}, nil
}

func (h *Handler) Register(mux *http.ServeMux) {
	// The body of a resolve-session request is a live credential. It is never logged, never
	// echoed into an error message, and must never be moved into the path or a query string.
	mux.HandleFunc("POST "+authapi.PathResolveSession, h.resolveSession)
	mux.HandleFunc("POST "+authapi.PathGetUser, h.getUser)
	mux.HandleFunc("POST "+authapi.PathListUsers, h.listUsers)
	mux.HandleFunc("POST "+authapi.PathUpdateProfile, h.updateProfile)
	mux.HandleFunc("POST "+authapi.PathDeleteUser, h.deleteUser)
	mux.HandleFunc("POST "+authapi.PathSetupOpen, h.setupOpen)
}

func (h *Handler) resolveSession(w http.ResponseWriter, r *http.Request) {
	var request authapi.ResolveSessionRequest
	if !httpx.DecodeJSON(w, r, &request) {
		return
	}

	_, user, err := h.service.ResolveSession(r.Context(), request.Token)
	h.user(w, r, "resolve session", user, err)
}

func (h *Handler) getUser(w http.ResponseWriter, r *http.Request) {
	var request authapi.GetUserRequest
	if !httpx.DecodeJSON(w, r, &request) {
		return
	}

	user, err := h.service.GetUserByRef(r.Context(), request.UserRef)
	h.user(w, r, "get user", user, err)
}

func (h *Handler) listUsers(w http.ResponseWriter, r *http.Request) {
	var request authapi.ListUsersRequest
	if !httpx.DecodeJSON(w, r, &request) {
		return
	}

	users, err := h.service.ListUsers(r.Context(), request.Limit)
	if err != nil {
		h.fail(w, r, "list users", err)

		return
	}

	wire := make([]authapi.User, 0, len(users))
	for _, user := range users {
		wire = append(wire, toWire(&user))
	}

	httpx.WriteJSON(w, authapi.ListUsersResponse{Users: wire})
}

func (h *Handler) updateProfile(w http.ResponseWriter, r *http.Request) {
	var request authapi.UpdateProfileRequest
	if !httpx.DecodeJSON(w, r, &request) {
		return
	}

	id, ok := h.userID(w, request.UserRef)
	if !ok {
		return
	}

	user, err := h.service.UpdateProfile(r.Context(), auth.UpdateProfileRequest{
		UserID:   id,
		Username: request.Username,
		Name:     request.Name,
	})
	h.user(w, r, "update profile", user, err)
}

func (h *Handler) deleteUser(w http.ResponseWriter, r *http.Request) {
	var request authapi.DeleteUserRequest
	if !httpx.DecodeJSON(w, r, &request) {
		return
	}

	id, ok := h.userID(w, request.UserRef)
	if !ok {
		return
	}

	if err := h.service.DeleteUser(r.Context(), id); err != nil {
		h.fail(w, r, "delete user", err)

		return
	}

	httpx.WriteJSON(w, authapi.Empty{})
}

func (h *Handler) setupOpen(w http.ResponseWriter, r *http.Request) {
	var request authapi.Empty
	if !httpx.DecodeJSON(w, r, &request) {
		return
	}

	open, err := h.service.SetupOpen(r.Context())
	if err != nil {
		h.fail(w, r, "setup open", err)

		return
	}

	httpx.WriteJSON(w, authapi.SetupOpenResponse{Open: open})
}

// userID unwraps a reference into the identifier the service takes. A reference that is not one
// names no account, which is the same answer as an account that does not exist.
func (h *Handler) userID(w http.ResponseWriter, userRef string) (string, bool) {
	parsed, err := ref.Parse(userRef)
	if err != nil {
		httpx.WriteError(w, httpx.CodeNotFound, "That account could not be found.")

		return "", false
	}

	return parsed.ID, true
}

func (h *Handler) user(w http.ResponseWriter, r *http.Request, operation string, user *auth.User, err error) {
	if err != nil {
		h.fail(w, r, operation, err)

		return
	}

	httpx.WriteJSON(w, authapi.UserResponse{User: toWire(user)})
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, operation string, err error) {
	if code, message, ok := Classify(err); ok {
		httpx.WriteError(w, code, message)

		return
	}

	httpx.WriteInternal(r.Context(), w, h.logger, operation, err)
}

// toWire carries the four fields a consumer needs. The password hash and the timestamps stay in
// this service.
func toWire(user *auth.User) authapi.User {
	return authapi.User{
		Ref:      user.Ref(),
		ID:       user.ID,
		Username: user.Username,
		Name:     user.Name,
	}
}
