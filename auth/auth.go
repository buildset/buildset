// Package auth owns identity: users, their credentials, and their sessions. It is the only place
// in the system that handles a password, and the only place that can turn a session token into a
// user. Other services hold a user reference and nothing else.
package auth

import "errors"

const (
	// ServiceName is the service segment of every reference this package owns.
	ServiceName = "auth"
	// UserResourceType is the type segment of a user reference.
	UserResourceType = "user"
)

var (
	// ErrInvalidCredentials covers both an unknown username and a wrong password. Telling the two
	// apart would turn the login form into a username oracle.
	ErrInvalidCredentials = errors.New("invalid username or password")

	ErrUserNotFound = errors.New("user not found")
	// ErrInvalidUsername wraps every username rule failure, so callers can show the reason without
	// matching on message text.
	ErrInvalidUsername = errors.New("invalid username")
	ErrUsernameTaken   = errors.New("username is already taken")
	ErrSessionNotFound = errors.New("session not found")

	// ErrSetupClosed is returned once the first user exists. Setup never reopens.
	ErrSetupClosed = errors.New("setup is already complete")
)
