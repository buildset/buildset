// Package postgres stores authz's roles and grants in Postgres. It owns its schema and migrates
// itself, so wiring authz to a different backend runs none of this.
//
// It is the twin of authz/sqlite and deliberately mirrors it statement for statement: the two are
// held to the same behaviour by the shared suite in authz/repotest.
package postgres

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"time"

	"github.com/buildset/buildset/authz"
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

func NewRepository(ctx context.Context, db *sql.DB) (*Repository, error) {
	runner := sqlmigrate.Runner{
		FileSystem: migrations,
		Directory:  "migrations",
		TableName:  "authz_schema_migrations",
		Dialect:    sqlmigrate.Postgres{},
	}

	if err := runner.Up(ctx, db); err != nil {
		return nil, fmt.Errorf("migrate authz schema: %w", err)
	}

	return &Repository{db: db}, nil
}

func (r *Repository) ListRoles(ctx context.Context) ([]authz.Role, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT name, description FROM roles ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("select roles: %w", err)
	}
	defer rows.Close()

	var roles []authz.Role

	for rows.Next() {
		var role authz.Role

		if err := rows.Scan(&role.Name, &role.Description); err != nil {
			return nil, fmt.Errorf("scan role: %w", err)
		}

		roles = append(roles, role)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate roles: %w", err)
	}

	return roles, nil
}

func (r *Repository) RoleExists(ctx context.Context, role string) (bool, error) {
	var exists int

	err := r.db.QueryRowContext(ctx, `SELECT 1 FROM roles WHERE name = $1`, role).Scan(&exists)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}

		return false, fmt.Errorf("select role: %w", err)
	}

	return true, nil
}

// SubjectPatterns unions the subject's role permissions with its direct grants. One query keeps
// this to a single round trip on a connection the whole process shares.
func (r *Repository) SubjectPatterns(ctx context.Context, subject string) ([]authz.Pattern, error) {
	const query = `
		SELECT role_permissions.action, role_permissions.resource_pattern
		FROM subject_roles
		JOIN role_permissions ON role_permissions.role = subject_roles.role
		WHERE subject_roles.subject_ref = $1
		UNION
		SELECT action, resource_ref FROM grants WHERE subject_ref = $2`

	rows, err := r.db.QueryContext(ctx, query, subject, subject)
	if err != nil {
		return nil, fmt.Errorf("select subject patterns: %w", err)
	}
	defer rows.Close()

	var patterns []authz.Pattern

	for rows.Next() {
		var pattern authz.Pattern

		if err := rows.Scan(&pattern.Action, &pattern.Resource); err != nil {
			return nil, fmt.Errorf("scan pattern: %w", err)
		}

		patterns = append(patterns, pattern)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate patterns: %w", err)
	}

	return patterns, nil
}

func (r *Repository) SubjectRoles(ctx context.Context, subject string) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT role FROM subject_roles WHERE subject_ref = $1 ORDER BY role`, subject)
	if err != nil {
		return nil, fmt.Errorf("select subject roles: %w", err)
	}
	defer rows.Close()

	var roles []string

	for rows.Next() {
		var role string

		if err := rows.Scan(&role); err != nil {
			return nil, fmt.Errorf("scan subject role: %w", err)
		}

		roles = append(roles, role)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate subject roles: %w", err)
	}

	return roles, nil
}

// InsertSubjectRole is idempotent, so assigning a role twice is not an error.
func (r *Repository) InsertSubjectRole(ctx context.Context, subject, role string, grantedAt time.Time) error {
	const query = `INSERT INTO subject_roles (subject_ref, role, granted_at) VALUES ($1, $2, $3)
		ON CONFLICT (subject_ref, role) DO NOTHING`

	if _, err := r.db.ExecContext(ctx, query, subject, role, formatTime(grantedAt)); err != nil {
		if isForeignKeyViolation(err) {
			return authz.ErrUnknownRole
		}

		return fmt.Errorf("insert subject role: %w", err)
	}

	return nil
}

func (r *Repository) DeleteSubjectRole(ctx context.Context, subject, role string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM subject_roles WHERE subject_ref = $1 AND role = $2`, subject, role)
	if err != nil {
		return fmt.Errorf("delete subject role: %w", err)
	}

	return nil
}

// InsertGrants writes every action for one resource in a single transaction, so a caller granting
// ownership never ends up with half the actions.
func (r *Repository) InsertGrants(ctx context.Context, subject string, actions []string, resource string, grantedAt time.Time) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	const query = `INSERT INTO grants (subject_ref, action, resource_ref, granted_at) VALUES ($1, $2, $3, $4)
		ON CONFLICT (subject_ref, action, resource_ref) DO NOTHING`

	for _, action := range actions {
		if _, err := tx.ExecContext(ctx, query, subject, action, resource, formatTime(grantedAt)); err != nil {
			return fmt.Errorf("insert grant %q: %w", action, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}

func (r *Repository) DeleteBySubject(ctx context.Context, subject string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM grants WHERE subject_ref = $1`, subject); err != nil {
		return fmt.Errorf("delete grants of subject: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM subject_roles WHERE subject_ref = $1`, subject); err != nil {
		return fmt.Errorf("delete roles of subject: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}

func (r *Repository) DeleteByResource(ctx context.Context, resource string) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM grants WHERE resource_ref = $1`, resource); err != nil {
		return fmt.Errorf("delete grants of resource: %w", err)
	}

	return nil
}

func formatTime(t time.Time) string {
	return t.UTC().Format(timeFormat)
}

var _ authz.Repository = (*Repository)(nil)
