// Package action holds the few permission strings that more than one binary names. The rest of the
// vocabulary stays in web; these two are here because the identity service asks who may reach its
// registration form and must not import the site to do so.
package action

const (
	// UserCreate is the permission to add an account.
	UserCreate = "user.create"
	// AnyUser is the resource pattern for a question about accounts in general.
	AnyUser = "urn:auth:user:*"
)
