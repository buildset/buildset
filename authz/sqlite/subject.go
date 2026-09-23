package sqlite

import (
	"context"
	"fmt"
	"time"

	"github.com/Masterminds/squirrel"
	"github.com/buildset/buildset/authz"
)

const tableSubjectRoles = "subject_roles"

const (
	subjectRoleColumnSubjectRef = "subject_ref"
	subjectRoleColumnRole       = "role"
	subjectRoleColumnGrantedAt  = "granted_at"
)

func subjectRoleColumns() []string {
	return []string{subjectRoleColumnSubjectRef, subjectRoleColumnRole, subjectRoleColumnGrantedAt}
}

// SubjectPatterns unions the subject's role permissions with its direct grants. Two queries would
// cost a round trip and lose the deduplication UNION gives.
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
		Values(subject, role, formatTime(grantedAt)).
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
