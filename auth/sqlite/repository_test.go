package sqlite_test

import (
	"database/sql"
	"testing"

	"github.com/buildset/buildset/auth"
	"github.com/buildset/buildset/auth/repotest"
	"github.com/buildset/buildset/auth/sqlite"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func TestRepository(t *testing.T) {
	repotest.Run(t, func(t *testing.T) auth.Repository {
		t.Helper()

		// foreign_keys is off by default in SQLite, and the cascade from users to sessions is part
		// of the contract, so the test database is opened the same way the application opens its own.
		db, err := sql.Open("sqlite", "file:"+t.TempDir()+"/test.db?_pragma=foreign_keys(ON)")
		require.NoError(t, err)

		db.SetMaxOpenConns(1)
		t.Cleanup(func() { _ = db.Close() })

		repository, err := sqlite.NewRepository(t.Context(), db)
		require.NoError(t, err)

		return repository
	})
}
