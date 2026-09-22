package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/buildset/buildset/pkg/storage"
)

// Stores holds one handle per service. Each service owns its storage: under SQLite they happen to
// be the same file, under Postgres they are separate pools with separate schemas, and no service
// ever learns which it got.
//
// They are separate because search_path is a property of a connection and connections are pooled,
// so three schemas cannot share one pool.
type Stores struct {
	Auth    *sql.DB
	Authz   *sql.DB
	Content *sql.DB

	closers []*sql.DB
}

// OpenStores connects each service's storage. It runs no migrations: each backend owns its own
// schema and migrates itself when it is wired up.
func OpenStores(ctx context.Context, cfg DatabaseConfig) (*Stores, error) {
	if cfg.Driver == storage.DriverSQLite {
		db, err := storage.Open(ctx, cfg, "")
		if err != nil {
			return nil, err
		}

		// One file for all three. The boundary between them is a convention here, not something
		// the database enforces; running the services apart is what makes it real.
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

// Ping reports whether every store is reachable. It backs the readiness probe.
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
