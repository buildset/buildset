package authz

import "time"

type Role struct {
	Name        string
	Description string
}

// Pattern is one permission: an action, possibly a wildcard, over a resource, possibly a wildcard.
type Pattern struct {
	Action   string
	Resource string
}

// Grant is a permission held by one subject over one resource.
type Grant struct {
	Subject   string
	Action    string
	Resource  string
	GrantedAt time.Time
}
