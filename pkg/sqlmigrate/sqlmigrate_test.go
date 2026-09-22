package sqlmigrate_test

import (
	"database/sql"
	"testing"
	"testing/fstest"

	"github.com/buildset/buildset/pkg/sqlmigrate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func newDatabase(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", "file:"+t.Name()+"?mode=memory&cache=shared")
	require.NoError(t, err)

	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	return db
}

func newRunner(files fstest.MapFS) sqlmigrate.Runner {
	return sqlmigrate.Runner{FileSystem: files, Directory: "migrations", TableName: "test_schema_migrations"}
}

func TestUp(t *testing.T) {
	ctx := t.Context()
	db := newDatabase(t)

	files := fstest.MapFS{
		"migrations/0002_widgets.sql": {Data: []byte(`CREATE TABLE widgets (id TEXT NOT NULL PRIMARY KEY) STRICT;`)},
		"migrations/0001_gadgets.sql": {Data: []byte("CREATE TABLE gadgets (id TEXT NOT NULL PRIMARY KEY) STRICT;\nINSERT INTO gadgets (id) VALUES ('first');")},
	}

	require.NoError(t, newRunner(files).Up(ctx, db))

	applied, err := newRunner(files).Applied(ctx, db)
	require.NoError(t, err)
	require.Len(t, applied, 2)
	assert.Equal(t, 1, applied[0].Version)
	assert.Equal(t, "gadgets", applied[0].Name)
	assert.Equal(t, 2, applied[1].Version)

	// The second statement of 0001 proves a multi-statement file applies as a whole.
	var count int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT count(*) FROM gadgets`).Scan(&count))
	assert.Equal(t, 1, count)

	// Running again is a no-op rather than a duplicate-table error.
	require.NoError(t, newRunner(files).Up(ctx, db))

	applied, err = newRunner(files).Applied(ctx, db)
	require.NoError(t, err)
	assert.Len(t, applied, 2)
}

func TestUpRejectsChangedFile(t *testing.T) {
	ctx := t.Context()
	db := newDatabase(t)

	files := fstest.MapFS{
		"migrations/0001_gadgets.sql": {Data: []byte(`CREATE TABLE gadgets (id TEXT NOT NULL PRIMARY KEY) STRICT;`)},
	}
	require.NoError(t, newRunner(files).Up(ctx, db))

	files["migrations/0001_gadgets.sql"] = &fstest.MapFile{Data: []byte(`CREATE TABLE gadgets (id TEXT NOT NULL PRIMARY KEY, extra TEXT) STRICT;`)}

	require.ErrorIs(t, newRunner(files).Up(ctx, db), sqlmigrate.ErrChecksumMismatch)
}

func TestUpRejectsMissingFile(t *testing.T) {
	ctx := t.Context()
	db := newDatabase(t)

	files := fstest.MapFS{
		"migrations/0001_gadgets.sql": {Data: []byte(`CREATE TABLE gadgets (id TEXT NOT NULL PRIMARY KEY) STRICT;`)},
	}
	require.NoError(t, newRunner(files).Up(ctx, db))

	delete(files, "migrations/0001_gadgets.sql")
	files["migrations/0002_widgets.sql"] = &fstest.MapFile{Data: []byte(`CREATE TABLE widgets (id TEXT NOT NULL PRIMARY KEY) STRICT;`)}

	require.ErrorIs(t, newRunner(files).Up(ctx, db), sqlmigrate.ErrMissingFile)
}

func TestUpLeavesNoRecordForFailedMigration(t *testing.T) {
	ctx := t.Context()
	db := newDatabase(t)

	files := fstest.MapFS{
		"migrations/0001_gadgets.sql": {Data: []byte(`CREATE TABLE gadgets (id TEXT NOT NULL PRIMARY KEY) STRICT;`)},
		"migrations/0002_broken.sql":  {Data: []byte(`THIS IS NOT SQL;`)},
	}

	require.Error(t, newRunner(files).Up(ctx, db))

	applied, err := newRunner(files).Applied(ctx, db)
	require.NoError(t, err)
	require.Len(t, applied, 1)
	assert.Equal(t, 1, applied[0].Version)
}

func TestUpRejectsBadNames(t *testing.T) {
	ctx := t.Context()
	db := newDatabase(t)

	files := fstest.MapFS{"migrations/create_gadgets.sql": {Data: []byte(`SELECT 1;`)}}
	require.Error(t, newRunner(files).Up(ctx, db))

	runner := sqlmigrate.Runner{
		FileSystem: fstest.MapFS{"migrations/0001_gadgets.sql": {Data: []byte(`SELECT 1;`)}},
		Directory:  "migrations",
		TableName:  "bad name; DROP TABLE users",
	}
	require.Error(t, runner.Up(ctx, db))
}
