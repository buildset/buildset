package web

import (
	"fmt"

	"github.com/nasermirzaei89/ms/pkg/ref"
)

// The actions this site asks about. The vocabulary belongs here: the authorization service stores
// these strings and attaches no meaning to them.
const (
	ActionPostCreate  = "post.create"
	ActionPostRead    = "post.read"
	ActionPostUpdate  = "post.update"
	ActionPostDelete  = "post.delete"
	ActionPostPublish = "post.publish"
	ActionUserRead    = "user.read"
	ActionUserCreate  = "user.create"
	ActionUserDelete  = "user.delete"
	ActionRoleAssign  = "role.assign"
)

// ownershipActions are granted to whoever creates a post. Ownership is expressed as grants rather
// than as an author comparison in a handler, so authorization stays in one place.
var ownershipActions = []string{ActionPostRead, ActionPostUpdate, ActionPostDelete, ActionPostPublish}

// administratorRole is the role this site refuses to let someone remove from themselves. The name
// is the application's, not the authorization service's.
const administratorRole = "admin"

// Wildcards for questions about a class of resource rather than one of them.
const (
	anyPostResource = "urn:content:post:*"
	// AnyUserResource is exported because the composition root asks the same question when it
	// decides who may reach the registration form.
	AnyUserResource = "urn:auth:user:*"
	anyUserResource = AnyUserResource
)

// postResource and userResource build a reference from an identifier taken out of the request
// path, so they must reject anything that is not one rather than panic on it.
func postResource(id string) (string, error) {
	resource, err := ref.New("content", "post", id)
	if err != nil {
		return "", fmt.Errorf("post id: %w", err)
	}

	return resource.String(), nil
}

func userResource(id string) (string, error) {
	resource, err := ref.New("auth", "user", id)
	if err != nil {
		return "", fmt.Errorf("user id: %w", err)
	}

	return resource.String(), nil
}
