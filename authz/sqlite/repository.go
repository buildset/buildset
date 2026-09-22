// Package sqlite stores authz's roles and grants in SQLite. It owns its schema and migrates
// itself, so wiring authz to a different backend runs none of this.
package sqlite

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
	sqlitedriver "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
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
		TableName:  "authz_schema_migrations",
	}

	if err := runner.Up(ctx, db); err != nil {
		return nil, fmt.Errorf("migrate authz schema: %w", err)
	}

	return &Repository{db: db}, nil
}

// placeholders is this backend's placeholder style, and the only place the dialect is named.
// SQLite takes ?, which is squirrel's default.
var placeholders squirrel.PlaceholderFormat = squirrel.Question

func builder() squirrel.StatementBuilderType {
	return squirrel.StatementBuilder.PlaceholderFormat(placeholders)
}

func isForeignKeyViolation(err error) bool {
	var sqliteError *sqlitedriver.Error

	return errors.As(err, &sqliteError) && sqliteError.Code() == sqlite3.SQLITE_CONSTRAINT_FOREIGNKEY
}

func formatTime(t time.Time) string {
	return t.UTC().Format(timeFormat)
}

var _ authz.Repository = (*Repository)(nil)
