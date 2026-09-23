// Package authzapi is the wire shape of the authorization service's API: types and paths only, so
// the service and its clients share one definition without any of them importing another.
package authzapi

const (
	PathCan           = "/v1/can"
	PathGrant         = "/v1/grant"
	PathAssignRole    = "/v1/assign-role"
	PathRevokeRole    = "/v1/revoke-role"
	PathSubjectRoles  = "/v1/subject-roles"
	PathListRoles     = "/v1/roles"
	PathPurgeResource = "/v1/purge-resource"
	PathPurgeSubject  = "/v1/purge-subject"
)

type CanRequest struct {
	Subject  string `json:"subject"`
	Action   string `json:"action"`
	Resource string `json:"resource"`
}

type CanResponse struct {
	Allowed bool `json:"allowed"`
}

type GrantRequest struct {
	Subject  string   `json:"subject"`
	Actions  []string `json:"actions"`
	Resource string   `json:"resource"`
}

type RoleRequest struct {
	Subject string `json:"subject"`
	Role    string `json:"role"`
}

type SubjectRequest struct {
	Subject string `json:"subject"`
}

type ResourceRequest struct {
	Resource string `json:"resource"`
}

// Role names only; a description has no consumer.
type RolesResponse struct {
	Roles []string `json:"roles"`
}

// Empty is the request or response of an operation that carries nothing.
type Empty struct{}
