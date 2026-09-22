package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/Masterminds/squirrel"
)

const tableGrants = "grants"

const (
	grantColumnSubjectRef  = "subject_ref"
	grantColumnAction      = "action"
	grantColumnResourceRef = "resource_ref"
	grantColumnGrantedAt   = "granted_at"
)

func grantColumns() []string {
	return []string{grantColumnSubjectRef, grantColumnAction, grantColumnResourceRef, grantColumnGrantedAt}
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
