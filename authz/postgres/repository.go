// Package postgres stores authz's roles and grants in Postgres. It owns its schema and migrates
// itself, so wiring authz to a different backend runs none of this.
//
// It is the twin of authz/sqlite. The two are held to the same behaviour by the shared suite in
// authz/repotest, and differ only in the placeholder style, the timestamp type, and how the driver
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
	"github.com/buildset/buildset/authz"
	"github.com/buildset/buildset/pkg/sqlmigrate"
)

//go:embed migrations/*.sql
var migrations embed.FS

const (
	tableRoles           = "roles"
	tableRolePermissions = "role_permissions"
	tableSubjectRoles    = "subject_roles"
	tableGrants          = "grants"
)

const (
	roleColumnName        = "name"
	roleColumnDescription = "description"

	permissionColumnRole            = "role"
	permissionColumnAction          = "action"
	permissionColumnResourcePattern = "resource_pattern"

	subjectRoleColumnSubjectRef = "subject_ref"
	subjectRoleColumnRole       = "role"
	subjectRoleColumnGrantedAt  = "granted_at"

	grantColumnSubjectRef  = "subject_ref"
	grantColumnAction      = "action"
	grantColumnResourceRef = "resource_ref"
	grantColumnGrantedAt   = "granted_at"
)

func roleColumns() []string {
	return []string{roleColumnName, roleColumnDescription}
}

func subjectRoleColumns() []string {
	return []string{subjectRoleColumnSubjectRef, subjectRoleColumnRole, subjectRoleColumnGrantedAt}
}

func grantColumns() []string {
	return []string{grantColumnSubjectRef, grantColumnAction, grantColumnResourceRef, grantColumnGrantedAt}
}

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

// placeholders is this backend's placeholder style, and the only place the dialect is named.
// Naming it once is what keeps every statement below identical to its SQLite twin.
var placeholders squirrel.PlaceholderFormat = squirrel.Dollar

func builder() squirrel.StatementBuilderType {
	return squirrel.StatementBuilder.PlaceholderFormat(placeholders)
}

func (r *Repository) ListRoles(ctx context.Context) ([]authz.Role, error) {
	rows, err := builder().RunWith(r.db).
		Select(roleColumns()...).
		From(tableRoles).
		OrderBy(roleColumnName).
		QueryContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("select roles: %w", err)
	}
	defer func() { _ = rows.Close() }()

	roles := make([]authz.Role, 0)

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

	err := builder().RunWith(r.db).
		Select("1").
		From(tableRoles).
		Where(squirrel.Eq{roleColumnName: role}).
		Limit(1).
		QueryRowContext(ctx).
		Scan(&exists)
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
//
// The union is written out rather than built, because a builder has no vocabulary for it and
// spelling it as two queries would cost a round trip and lose the deduplication UNION gives.
func (r *Repository) SubjectPatterns(ctx context.Context, subject string) ([]authz.Pattern, error) {
	roles := squirrel.
		Select(
			tableRolePermissions+"."+permissionColumnAction,
			tableRolePermissions+"."+permissionColumnResourcePattern,
		).
		From(tableSubjectRoles).
		Join(fmt.Sprintf("%[1]s ON %[1]s.%[2]s = %[3]s.%[4]s",
			tableRolePermissions, permissionColumnRole, tableSubjectRoles, subjectRoleColumnRole)).
		Where(squirrel.Eq{tableSubjectRoles + "." + subjectRoleColumnSubjectRef: subject})

	grants := squirrel.
		Select(grantColumnAction, grantColumnResourceRef).
		From(tableGrants).
		Where(squirrel.Eq{grantColumnSubjectRef: subject})

	rows, err := union(ctx, r.db, placeholders, roles, grants)
	if err != nil {
		return nil, fmt.Errorf("select subject patterns: %w", err)
	}
	defer func() { _ = rows.Close() }()

	patterns := make([]authz.Pattern, 0)

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
	rows, err := builder().RunWith(r.db).
		Select(subjectRoleColumnRole).
		From(tableSubjectRoles).
		Where(squirrel.Eq{subjectRoleColumnSubjectRef: subject}).
		OrderBy(subjectRoleColumnRole).
		QueryContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("select subject roles: %w", err)
	}
	defer func() { _ = rows.Close() }()

	roles := make([]string, 0)

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
	_, err := builder().RunWith(r.db).
		Insert(tableSubjectRoles).
		Columns(subjectRoleColumns()...).
		Values(subject, role, grantedAt).
		Suffix(fmt.Sprintf("ON CONFLICT (%s, %s) DO NOTHING", subjectRoleColumnSubjectRef, subjectRoleColumnRole)).
		ExecContext(ctx)
	if err != nil {
		if isForeignKeyViolation(err) {
			return fmt.Errorf("%w: %s", authz.ErrUnknownRole, role)
		}

		return fmt.Errorf("insert subject role: %w", err)
	}

	return nil
}

func (r *Repository) DeleteSubjectRole(ctx context.Context, subject, role string) error {
	_, err := builder().RunWith(r.db).
		Delete(tableSubjectRoles).
		Where(squirrel.Eq{subjectRoleColumnSubjectRef: subject, subjectRoleColumnRole: role}).
		ExecContext(ctx)
	if err != nil {
		return fmt.Errorf("delete subject role: %w", err)
	}

	return nil
}

// InsertGrants writes every action for one resource in a single transaction, so a caller granting
// ownership never ends up with half the actions.
func (r *Repository) InsertGrants(ctx context.Context, subject string, actions []string, resource string, grantedAt time.Time) error {
	if len(actions) == 0 {
		return nil
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	insert := builder().RunWith(tx).
		Insert(tableGrants).
		Columns(grantColumns()...)

	for _, action := range actions {
		insert = insert.Values(subject, action, resource, grantedAt)
	}

	insert = insert.Suffix(fmt.Sprintf("ON CONFLICT (%s, %s, %s) DO NOTHING",
		grantColumnSubjectRef, grantColumnAction, grantColumnResourceRef))

	if _, err := insert.ExecContext(ctx); err != nil {
		return fmt.Errorf("insert grants: %w", err)
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
	defer func() { _ = tx.Rollback() }()

	for _, table := range []struct {
		name   string
		column string
	}{
		{tableGrants, grantColumnSubjectRef},
		{tableSubjectRoles, subjectRoleColumnSubjectRef},
	} {
		_, err := builder().RunWith(tx).
			Delete(table.name).
			Where(squirrel.Eq{table.column: subject}).
			ExecContext(ctx)
		if err != nil {
			return fmt.Errorf("delete %s of subject: %w", table.name, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}

func (r *Repository) DeleteByResource(ctx context.Context, resource string) error {
	_, err := builder().RunWith(r.db).
		Delete(tableGrants).
		Where(squirrel.Eq{grantColumnResourceRef: resource}).
		ExecContext(ctx)
	if err != nil {
		return fmt.Errorf("delete grants of resource: %w", err)
	}

	return nil
}

var _ authz.Repository = (*Repository)(nil)
