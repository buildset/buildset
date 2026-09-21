package sqlmigrate

import (
	"context"
	"database/sql"
	"fmt"
	"hash/fnv"
)

// Dialect adapts Runner to one SQL engine. Everything that differs between engines is here: the
// bookkeeping DDL, its placeholder style, and a lock that keeps two processes from migrating at
// once. It is SQL text plus two calls, so this package still imports no driver.
type Dialect interface {
	// CreateTable is DDL that creates the bookkeeping table if it is absent.
	CreateTable(table string) string
	// Insert records one applied migration, taking version, name, checksum and applied_at.
	Insert(table string) string
	// Lock is held for the whole run. It may be a no-op where one is not needed.
	Lock(ctx context.Context, conn *sql.Conn, table string) error
	Unlock(ctx context.Context, conn *sql.Conn, table string) error
}

// SQLite is the default dialect, used when Runner.Dialect is nil.
type SQLite struct{}

func (SQLite) CreateTable(table string) string {
	return fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
		version    INTEGER NOT NULL PRIMARY KEY,
		name       TEXT NOT NULL,
		checksum   TEXT NOT NULL,
		applied_at TEXT NOT NULL
	) STRICT`, table)
}

func (SQLite) Insert(table string) string {
	return fmt.Sprintf(`INSERT INTO %s (version, name, checksum, applied_at) VALUES (?, ?, ?, ?)`, table)
}

// A SQLite database here is opened with a single connection and is not shared between processes,
// so the connection itself is already the lock.
func (SQLite) Lock(context.Context, *sql.Conn, string) error   { return nil }
func (SQLite) Unlock(context.Context, *sql.Conn, string) error { return nil }

// Postgres migrates under a session advisory lock, so replicas starting together serialise instead
// of racing to create the same tables.
type Postgres struct{}

func (Postgres) CreateTable(table string) string {
	return fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
		version    integer NOT NULL PRIMARY KEY,
		name       text NOT NULL,
		checksum   text NOT NULL,
		applied_at text NOT NULL
	)`, table)
}

func (Postgres) Insert(table string) string {
	return fmt.Sprintf(`INSERT INTO %s (version, name, checksum, applied_at) VALUES ($1, $2, $3, $4)`, table)
}

func (Postgres) Lock(ctx context.Context, conn *sql.Conn, table string) error {
	if _, err := conn.ExecContext(ctx, `SELECT pg_advisory_lock($1)`, advisoryKey(table)); err != nil {
		return fmt.Errorf("acquire advisory lock for %s: %w", table, err)
	}

	return nil
}

func (Postgres) Unlock(ctx context.Context, conn *sql.Conn, table string) error {
	if _, err := conn.ExecContext(ctx, `SELECT pg_advisory_unlock($1)`, advisoryKey(table)); err != nil {
		return fmt.Errorf("release advisory lock for %s: %w", table, err)
	}

	return nil
}

// advisoryKey derives the lock from the bookkeeping table name, which is the granularity wanted:
// two services in one database hold different locks, two replicas of one service hold the same.
// It only has to be stable across processes and releases.
func advisoryKey(table string) int64 {
	hash := fnv.New64a()
	hash.Write([]byte("sqlmigrate:" + table))

	return int64(hash.Sum64())
}
