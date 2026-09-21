package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
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
	switch cfg.Driver {
	case DriverSQLite:
		db, err := openSQLite(ctx, cfg)
		if err != nil {
			return nil, err
		}

		// One file for all three. The boundary between them is a convention here, not something
		// the database enforces; running the services apart is what makes it real.
		return &Stores{Auth: db, Authz: db, Content: db, closers: []*sql.DB{db}}, nil

	case DriverPostgres:
		stores := &Stores{}

		for _, target := range []struct {
			schema string
			handle **sql.DB
		}{
			{"auth", &stores.Auth},
			{"authz", &stores.Authz},
			{"content", &stores.Content},
		} {
			db, err := openPostgres(ctx, cfg, target.schema)
			if err != nil {
				stores.Close()

				return nil, err
			}

			*target.handle = db
			stores.closers = append(stores.closers, db)
		}

		return stores, nil

	default:
		return nil, fmt.Errorf("unknown database driver %q", cfg.Driver)
	}
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

func openSQLite(ctx context.Context, cfg DatabaseConfig) (*sql.DB, error) {
	dsn := "file:" + url.PathEscape(cfg.Path) +
		"?_pragma=busy_timeout(5000)" +
		"&_pragma=journal_mode(WAL)" +
		"&_pragma=foreign_keys(ON)" +
		"&_pragma=synchronous(NORMAL)"

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database %q: %w", cfg.Path, err)
	}

	// A single connection serializes every statement, which removes SQLITE_BUSY entirely.
	// TODO: split into a read pool plus one write connection if read latency becomes contended.
	db.SetMaxOpenConns(1)

	if err := db.PingContext(ctx); err != nil {
		db.Close()

		return nil, fmt.Errorf("ping sqlite database %q: %w", cfg.Path, err)
	}

	return db, nil
}

func openPostgres(ctx context.Context, cfg DatabaseConfig, schema string) (*sql.DB, error) {
	dsn, err := serviceDSN(cfg.DSN, schema)
	if err != nil {
		return nil, err
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open postgres schema %q: %w", schema, err)
	}

	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxOpenConns)
	// Bounded lifetimes so a Postgres restart or a pooler reload does not strand connections.
	db.SetConnMaxLifetime(30 * time.Minute)
	db.SetConnMaxIdleTime(5 * time.Minute)

	if err := db.PingContext(ctx); err != nil {
		db.Close()

		return nil, fmt.Errorf("ping postgres schema %q: %w", schema, err)
	}

	return db, nil
}

// serviceDSN points a connection at one service's schema. A deployment that gives each service its
// own role already sets search_path on that role, and this leaves such a DSN alone; it is here so
// one connection string also works, for a single binary and for tests.
func serviceDSN(dsn, schema string) (string, error) {
	parsed, err := url.Parse(dsn)
	if err != nil {
		return "", fmt.Errorf("parse DATABASE_DSN: %w", err)
	}

	query := parsed.Query()
	if query.Get("search_path") == "" {
		query.Set("search_path", schema)
		parsed.RawQuery = query.Encode()
	}

	return parsed.String(), nil
}
