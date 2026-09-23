package web

import (
	"fmt"

	"github.com/buildset/buildset/pkg/action"
	"github.com/buildset/buildset/pkg/ref"
)

// The actions this site asks about. The authorization service stores these strings and attaches no
// meaning to them.
const (
	ActionPostCreate  = "post.create"
	ActionPostRead    = "post.read"
	ActionPostUpdate  = "post.update"
	ActionPostDelete  = "post.delete"
	ActionPostPublish = "post.publish"
	ActionUserRead    = "user.read"
	ActionUserCreate  = action.UserCreate
	ActionUserDelete  = "user.delete"
	ActionRoleAssign  = "role.assign"
)

// ownershipActions are granted to whoever creates a post, so ownership stays a matter of grants
// rather than an author comparison in a handler.
var ownershipActions = []string{
	ActionPostRead,
	ActionPostUpdate,
	ActionPostDelete,
	ActionPostPublish,
}

// administratorRole is the role this site refuses to let someone remove from themselves.
const administratorRole = "admin"

// Wildcards for questions about a class of resource rather than one of them.
const (
	anyPostResource = "urn:content:post:*"
	// AnyUserResource is exported because the composition root asks the same question when it
	// decides who may reach the registration form.
	AnyUserResource = action.AnyUser
	anyUserResource = AnyUserResource
)

// The identifier comes out of the request path, so anything malformed must be rejected, not panic.
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
