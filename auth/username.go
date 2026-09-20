package auth

import (
	"fmt"
	"strings"
)

const (
	MinUsernameLength = 3
	MaxUsernameLength = 32
)

// NormalizeUsername is applied before every lookup and every insert, so "Ada" and "ada" are the
// same account rather than two.
func NormalizeUsername(username string) string {
	return strings.ToLower(strings.TrimSpace(username))
}

// ValidateUsername accepts a normalized username.
//
// TODO: usernames are not NFKC-normalized or checked for confusables (UTS #39), so two visually
// identical names can coexist. That matters once usernames are shown as an identity claim.
func ValidateUsername(username string) error {
	if len(username) < MinUsernameLength || len(username) > MaxUsernameLength {
		return fmt.Errorf("%w: it must be between %d and %d characters", ErrInvalidUsername, MinUsernameLength, MaxUsernameLength)
	}

	for i, c := range username {
		switch {
		case c >= 'a' && c <= 'z':
		case c >= '0' && c <= '9':
		case (c == '-' || c == '_') && i > 0:
		default:
			return fmt.Errorf("%w: it may contain only lowercase letters, digits, hyphen, and underscore, and must not start with a hyphen or underscore", ErrInvalidUsername)
		}
	}

	return nil
}
