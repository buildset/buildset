// Package pgtest gives the Postgres repository tests a database to run against.
//
// It starts one container per test binary and hands each test its own schema, so tests stay
// isolated without a container each and without truncating between them. It exists only for tests.
package pgtest

import (
	"context"
	"database/sql"
	"net/url"
	"regexp"
	"strconv"
	"sync"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	postgrestc "github.com/testcontainers/testcontainers-go/modules/postgres"
)

// image is pinned so a test failure is a change in this repository rather than in a tag.
const image = "postgres:18-alpine"

var schemaNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

var (
	containerOnce sync.Once
	containerDSN  string
	containerErr  error
)

// DSN returns a database these tests may create and drop schemas in, starting the container on
// first use. Every test package that calls it gets one container, and the reaper removes it when
// the test binary exits.
func DSN(t *testing.T) string {
	t.Helper()

	containerOnce.Do(func() {
		// Deliberately not t.Context: the container outlives the test that happened to start it.
		ctx := context.Background()

		container, err := postgrestc.Run(ctx, image,
			postgrestc.WithDatabase("buildset_test"),
			postgrestc.WithUsername("postgres"),
			postgrestc.WithPassword("postgres"),
			// C collation, so text identifiers order by byte exactly as they do under SQLite.
			// The repositories and their shared conformance suite depend on that.
			testcontainers.WithEnv(map[string]string{
				"POSTGRES_INITDB_ARGS": "--locale=C --encoding=UTF8",
			}),
			postgrestc.BasicWaitStrategies(),
		)
		if err != nil {
			containerErr = err

			return
		}

		containerDSN, containerErr = container.ConnectionString(ctx, "sslmode=disable")
	})

	require.NoError(t, containerErr, "start postgres container")

	return containerDSN
}

// Open creates an empty schema and returns a pool whose search_path is that schema alone. The
// schema is dropped when the test finishes, so tests can run in parallel.
func Open(t *testing.T, dsn string) *sql.DB {
	t.Helper()

	schema := "test_" + strconv.FormatInt(time.Now().UnixNano(), 36)
	require.Regexp(t, schemaNamePattern, schema, "generated schema name must be an identifier")

	admin, err := sql.Open("pgx", dsn)
	require.NoError(t, err)

	defer func() { _ = admin.Close() }()

	_, err = admin.ExecContext(t.Context(), `CREATE SCHEMA `+schema)
	require.NoError(t, err, "create schema %s", schema)

	scoped, err := sql.Open("pgx", withSearchPath(t, dsn, schema))
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = scoped.Close()

		cleanup, err := sql.Open("pgx", dsn)
		if err != nil {
			t.Logf("reopen to drop schema %s: %v", schema, err)

			return
		}
		defer func() { _ = cleanup.Close() }()

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
	require.NoError(t, err, "parse container dsn")

	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()

	return parsed.String()
}
