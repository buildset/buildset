package sqlmigrate_test

import (
	"sync"
	"testing"
	"testing/fstest"

	"github.com/buildset/buildset/pkg/pgtest"
	"github.com/buildset/buildset/pkg/sqlmigrate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newPostgresRunner(files fstest.MapFS) sqlmigrate.Runner {
	return sqlmigrate.Runner{
		FileSystem: files,
		Directory:  "migrations",
		TableName:  "test_schema_migrations",
		Dialect:    sqlmigrate.Postgres{},
	}
}

var postgresFiles = fstest.MapFS{
	// A multi-statement file is the case that matters: the whole file is sent as one string, which
	// only the simple query protocol accepts.
	"migrations/0001_gadgets.sql": {Data: []byte(
		"CREATE TABLE gadgets (id text NOT NULL PRIMARY KEY);\n" +
			"CREATE INDEX gadgets_id_idx ON gadgets (id);\n" +
			"INSERT INTO gadgets (id) VALUES ('first');")},
	"migrations/0002_widgets.sql": {
		Data: []byte(`CREATE TABLE widgets (id text NOT NULL PRIMARY KEY);`),
	},
}

func TestPostgresUp(t *testing.T) {
	ctx := t.Context()
	db := pgtest.Open(t, pgtest.DSN(t))

	require.NoError(t, newPostgresRunner(postgresFiles).Up(ctx, db))

	applied, err := newPostgresRunner(postgresFiles).Applied(ctx, db)
	require.NoError(t, err)
	require.Len(t, applied, 2)
	assert.Equal(t, 1, applied[0].Version)
	assert.Equal(t, "gadgets", applied[0].Name)
	assert.Equal(t, 2, applied[1].Version)
	assert.False(t, applied[0].AppliedAt.IsZero())

	var count int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT count(*) FROM gadgets`).Scan(&count))
	assert.Equal(t, 1, count)

	// Running again is a no-op rather than a duplicate-table error.
	require.NoError(t, newPostgresRunner(postgresFiles).Up(ctx, db))

	applied, err = newPostgresRunner(postgresFiles).Applied(ctx, db)
	require.NoError(t, err)
	assert.Len(t, applied, 2)
}

func TestPostgresUpRejectsAnEditedMigration(t *testing.T) {
	ctx := t.Context()
	db := pgtest.Open(t, pgtest.DSN(t))

	require.NoError(t, newPostgresRunner(postgresFiles).Up(ctx, db))

	edited := fstest.MapFS{
		"migrations/0001_gadgets.sql": {
			Data: []byte(`CREATE TABLE gadgets (id text NOT NULL PRIMARY KEY, extra text);`),
		},
		"migrations/0002_widgets.sql": postgresFiles["migrations/0002_widgets.sql"],
	}

	err := newPostgresRunner(edited).Up(ctx, db)
	require.ErrorIs(t, err, sqlmigrate.ErrChecksumMismatch)
}

func TestPostgresUpRefusesANewerSchema(t *testing.T) {
	ctx := t.Context()
	db := pgtest.Open(t, pgtest.DSN(t))

	require.NoError(t, newPostgresRunner(postgresFiles).Up(ctx, db))

	// A binary that knows only 0001 must not start against a database already carrying 0002.
	older := fstest.MapFS{
		"migrations/0001_gadgets.sql": postgresFiles["migrations/0001_gadgets.sql"],
	}

	err := newPostgresRunner(older).Up(ctx, db)
	require.ErrorIs(t, err, sqlmigrate.ErrMissingFile)
}

func TestPostgresUpSerialisesConcurrentRunners(t *testing.T) {
	ctx := t.Context()
	db := pgtest.Open(t, pgtest.DSN(t))

	// Two replicas starting together must not both try to create the same tables. The advisory
	// lock is what makes the loser wait rather than fail.
	const runners = 4

	errs := make([]error, runners)

	var group sync.WaitGroup

	group.Add(runners)

	for i := range runners {
		go func() {
			defer group.Done()

			errs[i] = newPostgresRunner(postgresFiles).Up(ctx, db)
		}()
	}

	group.Wait()

	for i, err := range errs {
		require.NoErrorf(t, err, "runner %d", i)
	}

	applied, err := newPostgresRunner(postgresFiles).Applied(ctx, db)
	require.NoError(t, err)
	assert.Len(t, applied, 2)

	var count int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT count(*) FROM gadgets`).Scan(&count))
	assert.Equal(t, 1, count, "0001 applied more than once")
}
