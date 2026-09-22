// Package sqlite stores content's posts in SQLite. It owns its schema and migrates itself, so
// wiring content to a different backend runs none of this.
package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"time"

	"github.com/Masterminds/squirrel"
	"github.com/buildset/buildset/content"
	"github.com/buildset/buildset/pkg/sqlmigrate"
)

//go:embed migrations/*.sql
var migrations embed.FS

const timeFormat = "2006-01-02T15:04:05.000Z"

type Repository struct {
	db *sql.DB
}

func NewRepository(ctx context.Context, db *sql.DB) (*Repository, error) {
	runner := sqlmigrate.Runner{
		FileSystem: migrations,
		Directory:  "migrations",
		TableName:  "content_schema_migrations",
	}

	if err := runner.Up(ctx, db); err != nil {
		return nil, fmt.Errorf("migrate content schema: %w", err)
	}

	return &Repository{db: db}, nil
}

// builder is the query builder bound to this backend. SQLite takes ? placeholders, which is
// squirrel's default, so this is the only place the dialect is named.
func (r *Repository) builder() squirrel.StatementBuilderType {
	return squirrel.StatementBuilder.RunWith(r.db)
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

func formatTime(t time.Time) string {
	return t.UTC().Format(timeFormat)
}

func formatOptionalTime(t *time.Time) any {
	if t == nil {
		return nil
	}

	return formatTime(*t)
}

func parseTime(value string) (time.Time, error) {
	return time.Parse(timeFormat, value)
}

var _ content.Repository = (*Repository)(nil)
