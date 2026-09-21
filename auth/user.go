package auth

import (
	"time"

	"github.com/buildset/buildset/pkg/ref"
)

type User struct {
	ID       string
	Username string
	Name     string
	// PasswordHash must never leave this service. Nothing outside auth has a reason to read it.
	PasswordHash string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (u *User) Ref() string {
	return ref.MustNew(ServiceName, UserResourceType, u.ID).String()
}

type Session struct {
	ID     string
	UserID string
	// TokenHash is the SHA-256 of the token handed to the browser. The token itself is never
	// stored, so a copy of the database does not hand over live sessions.
	TokenHash string
	CreatedAt time.Time
	// ExpiresAt is absolute, not sliding. A stolen token that is in constant use still dies.
	ExpiresAt time.Time
	LastSeen  time.Time
	UserAgent string
	IP        string
}

func (s *Session) Expired(now time.Time) bool {
	return !now.Before(s.ExpiresAt)
}
