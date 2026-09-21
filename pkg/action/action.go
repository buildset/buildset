// Package action holds the few permission strings that more than one binary names.
//
// The vocabulary belongs to the site, and the rest of it stays in web. These two are here because
// the identity service asks the authorization service who may reach its registration form, and it
// must not import the site to do so.
package action

const (
	// UserCreate is the permission to add an account.
	UserCreate = "user.create"
	// AnyUser is the resource pattern for a question about accounts in general.
	AnyUser = "urn:auth:user:*"
)
