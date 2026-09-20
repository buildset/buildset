package auth

import (
	"context"
	"time"
)

// Repository is the persistence contract. Implementations return ErrUserNotFound and
// ErrSessionNotFound for absent rows, and ErrUsernameTaken for a username collision.
type Repository interface {
	InsertUser(ctx context.Context, user *User) error
	UpdateUser(ctx context.Context, user *User) error
	DeleteUser(ctx context.Context, id string) error
	GetUser(ctx context.Context, id string) (*User, error)
	GetUserByUsername(ctx context.Context, username string) (*User, error)
	ListUsers(ctx context.Context, limit int) ([]User, error)
	CountUsers(ctx context.Context) (int, error)

	InsertSession(ctx context.Context, session *Session) error
	GetSessionByTokenHash(ctx context.Context, tokenHash string) (*Session, error)
	TouchSession(ctx context.Context, id string, lastSeen time.Time) error
	DeleteSessionByTokenHash(ctx context.Context, tokenHash string) error
	// DeleteSessionsByUser removes every session of a user except exceptSessionID, which may be
	// empty to remove all of them.
	DeleteSessionsByUser(ctx context.Context, userID, exceptSessionID string) error
	DeleteExpiredSessions(ctx context.Context, now time.Time) (int64, error)
}
