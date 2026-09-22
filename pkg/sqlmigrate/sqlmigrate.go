// Package sqlmigrate applies numbered SQL migration files from an fs.FS and records what it
// applied. Each storage backend owns its own files and its own bookkeeping table, so no two
// services ever write the same table.
//
// A file is applied inside one transaction together with its bookkeeping row, so a failure leaves
// nothing half-applied. That also means a statement which cannot run in a transaction, such as
// CREATE INDEX CONCURRENTLY, can never appear in a migration file.
package sqlmigrate

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"slices"
	"strconv"
	"time"
)

var (
	ErrChecksumMismatch = errors.New("applied migration has changed on disk")
	ErrMissingFile      = errors.New("applied migration is missing from disk")

	fileNamePattern  = regexp.MustCompile(`^(\d{4})_([a-z0-9_]+)\.sql$`)
	tableNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
)

// Runner applies the migrations of one storage backend.
type Runner struct {
	// FileSystem holds the migration files, normally an embed.FS.
	FileSystem fs.FS
	// Directory is the path within FileSystem holding NNNN_name.sql files.
	Directory string
	// TableName is where this backend records what it applied. It must be unique per service.
	TableName string
	// Dialect adapts the bookkeeping to the engine. Nil means SQLite.
	Dialect Dialect
}

func (r Runner) dialect() Dialect {
	if r.Dialect == nil {
		return SQLite{}
	}

	return r.Dialect
}

// queryer is the part of *sql.DB that this package uses, so the same code runs against a pool or
// against the single connection Up pins for the duration of a run.
type queryer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)
}

// Record is one applied migration.
type Record struct {
	Version   int
	Name      string
	Checksum  string
	AppliedAt time.Time
}

type migration struct {
	version    int
	name       string
	fileName   string
	checksum   string
	statements string
}

// Up applies every migration not yet recorded, in version order. It is forward-only: a bad
// migration is fixed by adding a new numbered file, never by editing an applied one.
func (r Runner) Up(ctx context.Context, db *sql.DB) error {
	if !tableNamePattern.MatchString(r.TableName) {
		return fmt.Errorf("invalid migration table name %q", r.TableName)
	}

	migrations, err := r.load()
	if err != nil {
		return err
	}

	// One connection for the whole run: a session advisory lock is only held by the session that
	// took it, so the lock and the statements it guards have to share a connection.
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("acquire connection: %w", err)
	}
	defer conn.Close()

	dialect := r.dialect()

	if err := dialect.Lock(ctx, conn, r.TableName); err != nil {
		return err
	}
	defer dialect.Unlock(ctx, conn, r.TableName)

	if err := r.createTable(ctx, conn); err != nil {
		return err
	}

	applied, err := r.applied(ctx, conn)
	if err != nil {
		return err
	}

	byVersion := make(map[int]Record, len(applied))
	for _, record := range applied {
		byVersion[record.Version] = record
	}

	for _, m := range migrations {
		record, ok := byVersion[m.version]
		if ok {
			if record.Checksum != m.checksum {
				return fmt.Errorf("%w: %s (recorded %s, found %s)", ErrChecksumMismatch, m.fileName, record.Checksum, m.checksum)
			}

			delete(byVersion, m.version)

			continue
		}

		if err := r.apply(ctx, conn, m); err != nil {
			return fmt.Errorf("apply migration %s: %w", m.fileName, err)
		}
	}

	// Anything still recorded has no file. Starting against a newer schema than the binary knows
	// about corrupts data quietly, so refuse instead.
	for version, record := range byVersion {
		return fmt.Errorf("%w: version %d (%s)", ErrMissingFile, version, record.Name)
	}

	return nil
}

// Applied returns the recorded migrations in version order.
func (r Runner) Applied(ctx context.Context, db *sql.DB) ([]Record, error) {
	return r.applied(ctx, db)
}

func (r Runner) applied(ctx context.Context, db queryer) ([]Record, error) {
	query := fmt.Sprintf(`SELECT version, name, checksum, applied_at FROM %s ORDER BY version`, r.TableName)

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("select applied migrations: %w", err)
	}
	defer rows.Close()

	var records []Record

	for rows.Next() {
		var (
			record    Record
			appliedAt any
		)

		if err := rows.Scan(&record.Version, &record.Name, &record.Checksum, &appliedAt); err != nil {
			return nil, fmt.Errorf("scan applied migration: %w", err)
		}

		record.AppliedAt, err = parseAppliedAt(appliedAt)
		if err != nil {
			return nil, fmt.Errorf("parse applied_at of version %d: %w", record.Version, err)
		}

		records = append(records, record)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate applied migrations: %w", err)
	}

	return records, nil
}

func (r Runner) createTable(ctx context.Context, db queryer) error {
	query := r.dialect().CreateTable(r.TableName)

	if _, err := db.ExecContext(ctx, query); err != nil {
		return fmt.Errorf("create migration table %s: %w", r.TableName, err)
	}

	return nil
}

func (r Runner) apply(ctx context.Context, db queryer, m migration) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, m.statements); err != nil {
		return fmt.Errorf("execute statements: %w", err)
	}

	insert := r.dialect().Insert(r.TableName)

	if _, err := tx.ExecContext(ctx, insert, m.version, m.name, m.checksum, r.dialect().AppliedAt(time.Now())); err != nil {
		return fmt.Errorf("record migration: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}

func (r Runner) load() ([]migration, error) {
	entries, err := fs.ReadDir(r.FileSystem, r.Directory)
	if err != nil {
		return nil, fmt.Errorf("read migration directory %q: %w", r.Directory, err)
	}

	migrations := make([]migration, 0, len(entries))
	seen := make(map[int]string, len(entries))

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		matches := fileNamePattern.FindStringSubmatch(entry.Name())
		if matches == nil {
			return nil, fmt.Errorf("migration file %q does not match NNNN_name.sql", entry.Name())
		}

		version, err := strconv.Atoi(matches[1])
		if err != nil {
			return nil, fmt.Errorf("parse version of %q: %w", entry.Name(), err)
		}

		if other, ok := seen[version]; ok {
			return nil, fmt.Errorf("migration version %d used by both %q and %q", version, other, entry.Name())
		}

		seen[version] = entry.Name()

		content, err := fs.ReadFile(r.FileSystem, path.Join(r.Directory, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("read migration %q: %w", entry.Name(), err)
		}

		sum := sha256.Sum256(content)

		migrations = append(migrations, migration{
			version:    version,
			name:       matches[2],
			fileName:   entry.Name(),
			checksum:   hex.EncodeToString(sum[:]),
			statements: string(content),
		})
	}

	slices.SortFunc(migrations, func(a, b migration) int { return a.version - b.version })

	return migrations, nil
}
