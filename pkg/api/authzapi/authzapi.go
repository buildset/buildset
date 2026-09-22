// Package authzapi is the wire shape of the authorization service's API.
//
// It holds types and paths and nothing else, so the service that serves the API and the clients
// that call it can share one definition without any of them importing another.
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

// RolesResponse carries role names only. A description has no consumer, and the in-process adapter
// has always dropped it too.
type RolesResponse struct {
	Roles []string `json:"roles"`
}

// Empty is the request or response of an operation that carries nothing.
type Empty struct{}
