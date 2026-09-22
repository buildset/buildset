// Package postgres stores auth's users and sessions in Postgres. It owns its schema and migrates
// itself, so wiring auth to a different backend runs none of this.
//
// It is the twin of auth/sqlite. The two are held to the same behaviour by the shared suite in
// auth/repotest, and differ only in the placeholder style, the timestamp type, and how the driver
// reports a constraint violation.
package postgres

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"time"

	"github.com/Masterminds/squirrel"
	"github.com/buildset/buildset/auth"
	"github.com/buildset/buildset/pkg/sqlmigrate"
)

//go:embed migrations/*.sql
var migrations embed.FS

const (
	tableUsers    = "users"
	tableSessions = "sessions"
)

const (
	userColumnID           = "id"
	userColumnUsername     = "username"
	userColumnName         = "name"
	userColumnPasswordHash = "password_hash"
	userColumnCreatedAt    = "created_at"
	userColumnUpdatedAt    = "updated_at"
)

func userColumns() []string {
	return []string{
		userColumnID,
		userColumnUsername,
		userColumnName,
		userColumnPasswordHash,
		userColumnCreatedAt,
		userColumnUpdatedAt,
	}
}

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

// builder is the query builder bound to this backend. Naming the placeholder style once here is
// what keeps every statement below identical to its SQLite twin.
func (r *Repository) builder() squirrel.StatementBuilderType {
	return squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar).RunWith(r.db)
}

func (r *Repository) InsertUser(ctx context.Context, user *auth.User) error {
	_, err := r.builder().
		Insert(tableUsers).
		Columns(userColumns()...).
		Values(
			user.ID,
			user.Username,
			user.Name,
			user.PasswordHash,
			user.CreatedAt,
			user.UpdatedAt,
		).
		ExecContext(ctx)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("%w: %s", auth.ErrUsernameTaken, user.Username)
		}

		return fmt.Errorf("insert user: %w", err)
	}

	return nil
}

func (r *Repository) UpdateUser(ctx context.Context, user *auth.User) error {
	result, err := r.builder().
		Update(tableUsers).
		Set(userColumnUsername, user.Username).
		Set(userColumnName, user.Name).
		Set(userColumnPasswordHash, user.PasswordHash).
		Set(userColumnUpdatedAt, user.UpdatedAt).
		Where(squirrel.Eq{userColumnID: user.ID}).
		ExecContext(ctx)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("%w: %s", auth.ErrUsernameTaken, user.Username)
		}

		return fmt.Errorf("update user: %w", err)
	}

	return requireOneRow(result, fmt.Errorf("%w: %s", auth.ErrUserNotFound, user.ID))
}

func (r *Repository) DeleteUser(ctx context.Context, id string) error {
	result, err := r.builder().
		Delete(tableUsers).
		Where(squirrel.Eq{userColumnID: id}).
		ExecContext(ctx)
	if err != nil {
		return fmt.Errorf("delete user: %w", err)
	}

	return requireOneRow(result, fmt.Errorf("%w: %s", auth.ErrUserNotFound, id))
}

func (r *Repository) GetUser(ctx context.Context, id string) (*auth.User, error) {
	row := r.builder().
		Select(userColumns()...).
		From(tableUsers).
		Where(squirrel.Eq{userColumnID: id}).
		QueryRowContext(ctx)

	return scanUser(row, fmt.Errorf("%w: %s", auth.ErrUserNotFound, id))
}

func (r *Repository) GetUserByUsername(ctx context.Context, username string) (*auth.User, error) {
	row := r.builder().
		Select(userColumns()...).
		From(tableUsers).
		Where(squirrel.Eq{userColumnUsername: username}).
		QueryRowContext(ctx)

	return scanUser(row, fmt.Errorf("%w: %s", auth.ErrUserNotFound, username))
}

func (r *Repository) ListUsers(ctx context.Context, limit int) ([]auth.User, error) {
	rows, err := r.builder().
		Select(userColumns()...).
		From(tableUsers).
		OrderBy(userColumnCreatedAt, userColumnID).
		Limit(uint64(limit)).
		QueryContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("select users: %w", err)
	}
	defer rows.Close()

	users := make([]auth.User, 0)

	for rows.Next() {
		user, err := scanUser(rows, auth.ErrUserNotFound)
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

	err := r.builder().
		Select("count(*)").
		From(tableUsers).
		QueryRowContext(ctx).
		Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count users: %w", err)
	}

	return count, nil
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

type rowScanner interface {
	Scan(dest ...any) error
}

func scanUser(row rowScanner, notFound error) (*auth.User, error) {
	var user auth.User

	err := row.Scan(&user.ID, &user.Username, &user.Name, &user.PasswordHash, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, notFound
		}

		return nil, fmt.Errorf("scan user: %w", err)
	}

	user.CreatedAt = user.CreatedAt.UTC()
	user.UpdatedAt = user.UpdatedAt.UTC()

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

var _ auth.Repository = (*Repository)(nil)
