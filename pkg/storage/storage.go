// Package storage opens a database for one service. It runs no migrations: each service's backend
// owns its own schema and migrates itself when it is wired up.
package storage

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"time"

	"github.com/nasermirzaei89/env"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

// Both are supported everywhere; SQLite is the default because it needs nothing to be running.
const (
	DriverSQLite   = "sqlite"
	DriverPostgres = "postgres"
)

type Config struct {
	// Driver is DriverSQLite or DriverPostgres.
	Driver string
	// Path is the SQLite file. The postgres driver ignores it.
	Path string
	// DSN is the Postgres connection string. The sqlite driver ignores it.
	DSN string
	// MaxOpenConns bounds each Postgres pool. SQLite is always one connection.
	MaxOpenConns int
}

func Load() Config {
	return Config{
		Driver:       env.GetString("DATABASE_DRIVER", DriverSQLite),
		Path:         env.GetString("DATABASE_PATH", "buildset.db"),
		DSN:          env.GetString("DATABASE_DSN", ""),
		MaxOpenConns: env.GetInt("DATABASE_MAX_OPEN_CONNS", 10),
	}
}

func (c Config) Validate() error {
	switch c.Driver {
	case DriverSQLite:
		if c.Path == "" {
			return fmt.Errorf("DATABASE_PATH must not be empty")
		}
	case DriverPostgres:
		if c.DSN == "" {
			return fmt.Errorf("DATABASE_DSN must not be empty when DATABASE_DRIVER is %q", DriverPostgres)
		}

		if c.MaxOpenConns < 1 {
			return fmt.Errorf("DATABASE_MAX_OPEN_CONNS must be positive, got %d", c.MaxOpenConns)
		}
	default:
		return fmt.Errorf("DATABASE_DRIVER must be %q or %q, got %q", DriverSQLite, DriverPostgres, c.Driver)
	}

	return nil
}

// schema names the Postgres schema this service owns, and is ignored by SQLite.
func Open(ctx context.Context, cfg Config, schema string) (*sql.DB, error) {
	switch cfg.Driver {
	case DriverSQLite:
		return openSQLite(ctx, cfg)
	case DriverPostgres:
		return openPostgres(ctx, cfg, schema)
	default:
		return nil, fmt.Errorf("unknown database driver %q", cfg.Driver)
	}
}

func openSQLite(ctx context.Context, cfg Config) (*sql.DB, error) {
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
		_ = db.Close()

		return nil, fmt.Errorf("ping sqlite database %q: %w", cfg.Path, err)
	}

	return db, nil
}

func openPostgres(ctx context.Context, cfg Config, schema string) (*sql.DB, error) {
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
		_ = db.Close()

		return nil, fmt.Errorf("ping postgres schema %q: %w", schema, err)
	}

	return db, nil
}

// serviceDSN points a connection at one service's schema, so one connection string works for a
// single binary and for tests. A DSN that already sets search_path is left alone.
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
