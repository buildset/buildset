// Package postgres stores content's posts in Postgres. It owns its schema and migrates itself, so
// wiring content to a different backend runs none of this.
//
// It is the twin of content/sqlite. The two are held to the same behaviour by the shared suite in
// content/repotest, and differ only in the placeholder style and the timestamp type.
package postgres

import (
	"context"
	"database/sql"
	"embed"
	"fmt"

	"github.com/Masterminds/squirrel"
	"github.com/buildset/buildset/content"
	"github.com/buildset/buildset/pkg/sqlmigrate"
)

//go:embed migrations/*.sql
var migrations embed.FS

type Repository struct {
	db *sql.DB
}

func NewRepository(ctx context.Context, db *sql.DB) (*Repository, error) {
	runner := sqlmigrate.Runner{
		FileSystem: migrations,
		Directory:  "migrations",
		TableName:  "content_schema_migrations",
		Dialect:    sqlmigrate.Postgres{},
	}

	if err := runner.Up(ctx, db); err != nil {
		return nil, fmt.Errorf("migrate content schema: %w", err)
	}

	return &Repository{db: db}, nil
}

// builder is the query builder bound to this backend. Naming the placeholder style once here is
// what keeps every statement in this package identical to its SQLite twin, including the filtered
// listing whose placeholders would otherwise have to be numbered by hand.
func (r *Repository) builder() squirrel.StatementBuilderType {
	return squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar).RunWith(r.db)
}

type rowScanner interface {
	Scan(dest ...any) error
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

var _ content.Repository = (*Repository)(nil)
