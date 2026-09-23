// Package authapi is the wire shape of the identity service's API: types and paths only, so the
// service and its client share one definition without either importing the other.
package authapi

// The operations the site needs. Registering, authenticating, session creation, password change and
// setup are deliberately absent: they belong to the identity service's own pages.
const (
	PathResolveSession = "/v1/resolve-session"
	PathGetUser        = "/v1/get-user"
	PathListUsers      = "/v1/list-users"
	PathUpdateProfile  = "/v1/update-profile"
	PathDeleteUser     = "/v1/delete-user"
	PathSetupOpen      = "/v1/setup-open"
)

// User carries no credentials and no timestamps: nothing outside the identity service reads them.
type User struct {
	Ref      string `json:"ref"`
	ID       string `json:"id"`
	Username string `json:"username"`
	Name     string `json:"name"`
}

// ResolveSessionRequest carries a live session token. It travels in the body so it cannot reach an
// access log, a Referer header or a proxy's error page. Nothing may log this struct.
type ResolveSessionRequest struct {
	Token string `json:"token"`
}

type UserResponse struct {
	User User `json:"user"`
}

type GetUserRequest struct {
	UserRef string `json:"user_ref"`
}

type ListUsersRequest struct {
	Limit int `json:"limit"`
}

type ListUsersResponse struct {
	Users []User `json:"users"`
}

type UpdateProfileRequest struct {
	UserRef  string `json:"user_ref"`
	Username string `json:"username"`
	Name     string `json:"name"`
}

type DeleteUserRequest struct {
	UserRef string `json:"user_ref"`
}

type SetupOpenResponse struct {
	Open bool `json:"open"`
}

// Empty answers a void operation, rather than 204, so success and failure decode through the same
// path.
type Empty struct{}
