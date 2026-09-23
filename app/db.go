package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/buildset/buildset/pkg/storage"
)

// Stores holds one handle per service. Under SQLite they are the same file; under Postgres they are
// separate pools because search_path belongs to a connection and connections are pooled.
type Stores struct {
	Auth    *sql.DB
	Authz   *sql.DB
	Content *sql.DB

	closers []*sql.DB
}

// OpenStores runs no migrations: each backend migrates itself when it is wired up.
func OpenStores(ctx context.Context, cfg DatabaseConfig) (*Stores, error) {
	if cfg.Driver == storage.DriverSQLite {
		db, err := storage.Open(ctx, cfg, "")
		if err != nil {
			return nil, err
		}

		// One file for all three; the boundary is a convention, not something the database enforces.
		return &Stores{Auth: db, Authz: db, Content: db, closers: []*sql.DB{db}}, nil
	}

	stores := &Stores{}

	for _, target := range []struct {
		schema string
		handle **sql.DB
	}{
		{"auth", &stores.Auth},
		{"authz", &stores.Authz},
		{"content", &stores.Content},
	} {
		db, err := storage.Open(ctx, cfg, target.schema)
		if err != nil {
			_ = stores.Close()

			return nil, err
		}

		*target.handle = db
		stores.closers = append(stores.closers, db)
	}

	return stores, nil
}

// Ping reports whether every store is reachable.
func (s *Stores) Ping(ctx context.Context) error {
	for _, db := range s.closers {
		if err := db.PingContext(ctx); err != nil {
			return fmt.Errorf("ping database: %w", err)
		}
	}

	return nil
}

func (s *Stores) Close() error {
	var errs []error

	for _, db := range s.closers {
		if err := db.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close database: %w", err))
		}
	}

	return errors.Join(errs...)
}
