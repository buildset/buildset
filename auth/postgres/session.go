package postgres

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
			session.CreatedAt,
			session.ExpiresAt,
			session.LastSeen,
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
	var session auth.Session

	err := r.builder().
		Select(sessionColumns()...).
		From(tableSessions).
		Where(squirrel.Eq{sessionColumnTokenHash: tokenHash}).
		QueryRowContext(ctx).
		Scan(
			&session.ID, &session.UserID, &session.TokenHash,
			&session.CreatedAt, &session.ExpiresAt, &session.LastSeen, &session.UserAgent, &session.IP,
		)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, auth.ErrSessionNotFound
		}

		return nil, fmt.Errorf("select session: %w", err)
	}

	// A timestamptz comes back in the session's time zone. These are instants to everything above
	// this layer, so they are normalised rather than carrying the server's zone around.
	session.CreatedAt = session.CreatedAt.UTC()
	session.ExpiresAt = session.ExpiresAt.UTC()
	session.LastSeen = session.LastSeen.UTC()

	return &session, nil
}

func (r *Repository) TouchSession(ctx context.Context, id string, lastSeen time.Time) error {
	_, err := r.builder().
		Update(tableSessions).
		Set(sessionColumnLastSeen, lastSeen).
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
		Where(squirrel.LtOrEq{sessionColumnExpiresAt: now}).
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
