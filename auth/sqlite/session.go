package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Masterminds/squirrel"
	"github.com/buildset/buildset/auth"
)

const tableSessions = "sessions"

const (
	sessionColumnID        = "id"
	sessionColumnUserID    = "user_id"
	sessionColumnTokenHash = "token_hash"
	sessionColumnCreatedAt = "created_at"
	sessionColumnExpiresAt = "expires_at"
	sessionColumnLastSeen  = "last_seen"
	sessionColumnUserAgent = "user_agent"
	sessionColumnIP        = "ip"
)

func sessionColumns() []string {
	return []string{
		sessionColumnID,
		sessionColumnUserID,
		sessionColumnTokenHash,
		sessionColumnCreatedAt,
		sessionColumnExpiresAt,
		sessionColumnLastSeen,
		sessionColumnUserAgent,
		sessionColumnIP,
	}
}
func (r *Repository) InsertSession(ctx context.Context, session *auth.Session) error {
	_, err := r.builder().
		Insert(tableSessions).
		Columns(sessionColumns()...).
		Values(
			session.ID,
			session.UserID,
			session.TokenHash,
			formatTime(session.CreatedAt),
			formatTime(session.ExpiresAt),
			formatTime(session.LastSeen),
			session.UserAgent,
			session.IP,
		).
		ExecContext(ctx)
	if err != nil {
		return fmt.Errorf("insert session: %w", err)
	}

	return nil
}

func (r *Repository) GetSessionByTokenHash(ctx context.Context, tokenHash string) (*auth.Session, error) {
	var (
		session                          auth.Session
		createdAt, expiresAt, lastSeenAt string
	)

	err := r.builder().
		Select(sessionColumns()...).
		From(tableSessions).
		Where(squirrel.Eq{sessionColumnTokenHash: tokenHash}).
		QueryRowContext(ctx).
		Scan(
			&session.ID, &session.UserID, &session.TokenHash,
			&createdAt, &expiresAt, &lastSeenAt, &session.UserAgent, &session.IP,
		)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, auth.ErrSessionNotFound
		}

		return nil, fmt.Errorf("select session: %w", err)
	}

	for _, field := range []struct {
		raw    string
		target *time.Time
	}{
		{createdAt, &session.CreatedAt},
		{expiresAt, &session.ExpiresAt},
		{lastSeenAt, &session.LastSeen},
	} {
		parsed, err := parseTime(field.raw)
		if err != nil {
			return nil, fmt.Errorf("parse session timestamp: %w", err)
		}

		*field.target = parsed
	}

	return &session, nil
}

func (r *Repository) TouchSession(ctx context.Context, id string, lastSeen time.Time) error {
	_, err := r.builder().
		Update(tableSessions).
		Set(sessionColumnLastSeen, formatTime(lastSeen)).
		Where(squirrel.Eq{sessionColumnID: id}).
		ExecContext(ctx)
	if err != nil {
		return fmt.Errorf("touch session: %w", err)
	}

	return nil
}

func (r *Repository) DeleteSessionByTokenHash(ctx context.Context, tokenHash string) error {
	_, err := r.builder().
		Delete(tableSessions).
		Where(squirrel.Eq{sessionColumnTokenHash: tokenHash}).
		ExecContext(ctx)
	if err != nil {
		return fmt.Errorf("delete session: %w", err)
	}

	return nil
}

func (r *Repository) DeleteSessionsByUser(ctx context.Context, userID, exceptSessionID string) error {
	// An empty exceptSessionID matches no row, so every session of the user is removed.
	_, err := r.builder().
		Delete(tableSessions).
		Where(squirrel.Eq{sessionColumnUserID: userID}).
		Where(squirrel.NotEq{sessionColumnID: exceptSessionID}).
		ExecContext(ctx)
	if err != nil {
		return fmt.Errorf("delete sessions of user: %w", err)
	}

	return nil
}

func (r *Repository) DeleteExpiredSessions(ctx context.Context, now time.Time) (int64, error) {
	result, err := r.builder().
		Delete(tableSessions).
		Where(squirrel.LtOrEq{sessionColumnExpiresAt: formatTime(now)}).
		ExecContext(ctx)
	if err != nil {
		return 0, fmt.Errorf("delete expired sessions: %w", err)
	}

	deleted, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("count deleted sessions: %w", err)
	}

	return deleted, nil
}
