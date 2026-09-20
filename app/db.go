package app

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"

	_ "modernc.org/sqlite"
)

// OpenDatabase opens the SQLite file and applies connection-wide pragmas. It runs no migrations:
// each service's storage backend owns its own schema and migrates itself when it is wired up.
func OpenDatabase(ctx context.Context, cfg DatabaseConfig) (*sql.DB, error) {
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
