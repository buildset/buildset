// Package authapi is the wire shape of the identity service's API.
//
// It holds types and paths and nothing else, so the service that serves the API and the client
// that calls it can share one definition without either importing the other.
package authapi

// The operations the site needs from the identity service. Registering, authenticating, creating a
// session, changing a password and completing setup are deliberately absent: they belong to the
// identity service's own pages, which run in the same process, so nothing on the network can
// create a session or change a password.
const (
	PathResolveSession = "/v1/resolve-session"
	PathGetUser        = "/v1/get-user"
	PathListUsers      = "/v1/list-users"
	PathUpdateProfile  = "/v1/update-profile"
	PathDeleteUser     = "/v1/delete-user"
	PathSetupOpen      = "/v1/setup-open"
)

// User is what a consumer needs to know about a person. It carries no credentials and no
// timestamps, because nothing outside the identity service has a reason to read them.
type User struct {
	Ref      string `json:"ref"`
	ID       string `json:"id"`
	Username string `json:"username"`
	Name     string `json:"name"`
}

// ResolveSessionRequest carries a live session token.
//
// It travels in the body rather than in the path or a query so that it cannot reach an access log,
// a Referer header or a proxy's error page. Nothing may log this struct.
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

// Empty is the request or response of an operation that carries nothing. Void operations answer
// with it rather than 204, so success and failure decode through the same path.
type Empty struct{}
