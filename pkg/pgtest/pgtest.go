// Package pgtest gives the Postgres repository tests a throwaway schema each. It exists only for
// tests, and is skipped entirely unless a database is offered through TEST_POSTGRES_DSN.
package pgtest

import (
	"database/sql"
	"net/url"
	"regexp"
	"strconv"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/nasermirzaei89/env"
)

// DSNEnvVar points at a database these tests may create and drop schemas in. Without it they skip,
// so the default `go test ./...` stays offline and fast.
const DSNEnvVar = "TEST_POSTGRES_DSN"

var schemaNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// DSN returns the configured database, or skips the test.
func DSN(t *testing.T) string {
	t.Helper()

	dsn := env.GetString(DSNEnvVar, "")
	if dsn == "" {
		t.Skipf("set %s to run the postgres tests", DSNEnvVar)
	}

	return dsn
}

// Open creates an empty schema and returns a pool whose search_path is that schema alone. The
// schema is dropped when the test finishes, so tests can run in parallel without truncating
// anything between them.
func Open(t *testing.T, dsn string) *sql.DB {
	t.Helper()

	schema := "test_" + strconv.FormatInt(time.Now().UnixNano(), 36)
	if !schemaNamePattern.MatchString(schema) {
		t.Fatalf("generated schema name %q is not an identifier", schema)
	}

	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open admin connection: %v", err)
	}
	defer admin.Close()

	if _, err := admin.ExecContext(t.Context(), `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create schema %s: %v", schema, err)
	}

	scoped, err := sql.Open("pgx", withSearchPath(t, dsn, schema))
	if err != nil {
		t.Fatalf("open scoped connection: %v", err)
	}

	t.Cleanup(func() {
		scoped.Close()

		cleanup, err := sql.Open("pgx", dsn)
		if err != nil {
			t.Logf("reopen to drop schema %s: %v", schema, err)

			return
		}
		defer cleanup.Close()

		// t.Context is already cancelled by the time cleanups run, so this uses its own.
		if _, err := cleanup.Exec(`DROP SCHEMA ` + schema + ` CASCADE`); err != nil {
			t.Logf("drop schema %s: %v", schema, err)
		}
	})

	return scoped
}

func withSearchPath(t *testing.T, dsn, schema string) string {
	t.Helper()

	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse %s: %v", DSNEnvVar, err)
	}

	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()

	return parsed.String()
}
