// Package postgres stores auth's users and sessions in Postgres. It owns its schema and migrates
// itself, so wiring auth to a different backend runs none of this.
//
// It is the twin of auth/sqlite and deliberately mirrors it statement for statement: the two are
// held to the same behaviour by the shared suite in auth/repotest.
package postgres

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"time"

	"github.com/buildset/buildset/auth"
	"github.com/buildset/buildset/pkg/sqlmigrate"
)

//go:embed migrations/*.sql
var migrations embed.FS

// timeFormat sorts lexicographically, so ordering and range queries work on the stored text. The
// columns holding it are COLLATE "C", which is what makes that true whatever the database locale.
//
// TODO: move to timestamptz. Every conversion funnels through formatTime and parseTime, so it is
// an ALTER per column plus the scan targets in this file.
const timeFormat = "2006-01-02T15:04:05.000Z"

type Repository struct {
	db *sql.DB
}

// NewRepository migrates the schema and returns a repository over it.
func NewRepository(ctx context.Context, db *sql.DB) (*Repository, error) {
	runner := sqlmigrate.Runner{
		FileSystem: migrations,
		Directory:  "migrations",
		TableName:  "auth_schema_migrations",
		Dialect:    sqlmigrate.Postgres{},
	}

	if err := runner.Up(ctx, db); err != nil {
		return nil, fmt.Errorf("migrate auth schema: %w", err)
	}

	return &Repository{db: db}, nil
}

func (r *Repository) InsertUser(ctx context.Context, user *auth.User) error {
	const query = `INSERT INTO users (id, username, name, password_hash, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)`

	_, err := r.db.ExecContext(ctx, query,
		user.ID, user.Username, user.Name, user.PasswordHash,
		formatTime(user.CreatedAt), formatTime(user.UpdatedAt),
	)
	if err != nil {
		if isUniqueViolation(err) {
			return auth.ErrUsernameTaken
		}

		return fmt.Errorf("insert user: %w", err)
	}

	return nil
}

func (r *Repository) UpdateUser(ctx context.Context, user *auth.User) error {
	const query = `UPDATE users SET username = $1, name = $2, password_hash = $3, updated_at = $4 WHERE id = $5`

	result, err := r.db.ExecContext(ctx, query,
		user.Username, user.Name, user.PasswordHash, formatTime(user.UpdatedAt), user.ID,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return auth.ErrUsernameTaken
		}

		return fmt.Errorf("update user: %w", err)
	}

	return requireOneRow(result, auth.ErrUserNotFound)
}

func (r *Repository) DeleteUser(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete user: %w", err)
	}

	return requireOneRow(result, auth.ErrUserNotFound)
}

func (r *Repository) GetUser(ctx context.Context, id string) (*auth.User, error) {
	const query = `SELECT id, username, name, password_hash, created_at, updated_at FROM users WHERE id = $1`

	return scanUser(r.db.QueryRowContext(ctx, query, id))
}

func (r *Repository) GetUserByUsername(ctx context.Context, username string) (*auth.User, error) {
	const query = `SELECT id, username, name, password_hash, created_at, updated_at FROM users WHERE username = $1`

	return scanUser(r.db.QueryRowContext(ctx, query, username))
}

func (r *Repository) ListUsers(ctx context.Context, limit int) ([]auth.User, error) {
	const query = `SELECT id, username, name, password_hash, created_at, updated_at FROM users
		ORDER BY created_at, id LIMIT $1`

	rows, err := r.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("select users: %w", err)
	}
	defer rows.Close()

	var users []auth.User

	for rows.Next() {
		user, err := scanUser(rows)
		if err != nil {
			return nil, err
		}

		users = append(users, *user)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate users: %w", err)
	}

	return users, nil
}

func (r *Repository) CountUsers(ctx context.Context) (int, error) {
	var count int

	if err := r.db.QueryRowContext(ctx, `SELECT count(*) FROM users`).Scan(&count); err != nil {
		return 0, fmt.Errorf("count users: %w", err)
	}

	return count, nil
}

func (r *Repository) InsertSession(ctx context.Context, session *auth.Session) error {
	const query = `INSERT INTO sessions (id, user_id, token_hash, created_at, expires_at, last_seen, user_agent, ip)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`

	_, err := r.db.ExecContext(ctx, query,
		session.ID, session.UserID, session.TokenHash,
		formatTime(session.CreatedAt), formatTime(session.ExpiresAt), formatTime(session.LastSeen),
		session.UserAgent, session.IP,
	)
	if err != nil {
		return fmt.Errorf("insert session: %w", err)
	}

	return nil
}

func (r *Repository) GetSessionByTokenHash(ctx context.Context, tokenHash string) (*auth.Session, error) {
	const query = `SELECT id, user_id, token_hash, created_at, expires_at, last_seen, user_agent, ip
		FROM sessions WHERE token_hash = $1`

	var (
		session                          auth.Session
		createdAt, expiresAt, lastSeenAt string
	)

	err := r.db.QueryRowContext(ctx, query, tokenHash).Scan(
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
	_, err := r.db.ExecContext(ctx, `UPDATE sessions SET last_seen = $1 WHERE id = $2`, formatTime(lastSeen), id)
	if err != nil {
		return fmt.Errorf("touch session: %w", err)
	}

	return nil
}

func (r *Repository) DeleteSessionByTokenHash(ctx context.Context, tokenHash string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = $1`, tokenHash)
	if err != nil {
		return fmt.Errorf("delete session: %w", err)
	}

	return nil
}

func (r *Repository) DeleteSessionsByUser(ctx context.Context, userID, exceptSessionID string) error {
	// An empty exceptSessionID matches no row, so every session of the user is removed.
	_, err := r.db.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = $1 AND id <> $2`, userID, exceptSessionID)
	if err != nil {
		return fmt.Errorf("delete sessions of user: %w", err)
	}

	return nil
}

func (r *Repository) DeleteExpiredSessions(ctx context.Context, now time.Time) (int64, error) {
	result, err := r.db.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at <= $1`, formatTime(now))
	if err != nil {
		return 0, fmt.Errorf("delete expired sessions: %w", err)
	}

	deleted, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("count deleted sessions: %w", err)
	}

	return deleted, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanUser(row rowScanner) (*auth.User, error) {
	var (
		user                 auth.User
		createdAt, updatedAt string
	)

	err := row.Scan(&user.ID, &user.Username, &user.Name, &user.PasswordHash, &createdAt, &updatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, auth.ErrUserNotFound
		}

		return nil, fmt.Errorf("scan user: %w", err)
	}

	if user.CreatedAt, err = parseTime(createdAt); err != nil {
		return nil, fmt.Errorf("parse user created_at: %w", err)
	}

	if user.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return nil, fmt.Errorf("parse user updated_at: %w", err)
	}

	return &user, nil
}

func requireOneRow(result sql.Result, notFound error) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count affected rows: %w", err)
	}

	if affected == 0 {
		return notFound
	}

	return nil
}

func formatTime(t time.Time) string {
	return t.UTC().Format(timeFormat)
}

func parseTime(value string) (time.Time, error) {
	return time.Parse(timeFormat, value)
}

var _ auth.Repository = (*Repository)(nil)
