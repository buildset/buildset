// Package postgres stores authz's roles and grants in Postgres. It owns its schema and migrates
// itself, so wiring authz to a different backend runs none of this.
package postgres

import (
	"context"
	"database/sql"
	"embed"
	"fmt"

	"github.com/Masterminds/squirrel"
	"github.com/buildset/buildset/authz"
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
		TableName:  "authz_schema_migrations",
		Dialect:    sqlmigrate.Postgres{},
	}

	if err := runner.Up(ctx, db); err != nil {
		return nil, fmt.Errorf("migrate authz schema: %w", err)
	}

	return &Repository{db: db}, nil
}

// placeholders is this package's placeholder style, and the only place the dialect is named.
var placeholders squirrel.PlaceholderFormat = squirrel.Dollar

func builder() squirrel.StatementBuilderType {
	return squirrel.StatementBuilder.PlaceholderFormat(placeholders)
}

var _ authz.Repository = (*Repository)(nil)
