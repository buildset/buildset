// Package authz answers one question: may this subject perform this action on this resource?
//
// Subjects and resources are opaque references. This service never resolves one, never stores
// anything about what a reference means, and has no idea that posts or users exist. That is what
// lets every other service delegate authorization here instead of reimplementing it.
package authz

import "errors"

var (
	ErrUnknownRole   = errors.New("unknown role")
	ErrInvalidAction = errors.New("invalid action")
)
