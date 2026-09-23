// Package authz answers one question: may this subject perform this action on this resource?
// Subjects and resources are opaque references that this service never resolves, so it has no idea
// that posts or users exist.
package authz

import "errors"

var (
	ErrUnknownRole   = errors.New("unknown role")
	ErrInvalidAction = errors.New("invalid action")
)
