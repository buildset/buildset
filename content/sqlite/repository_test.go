package sqlite_test

import (
	"database/sql"
	"testing"

	"github.com/buildset/buildset/content"
	"github.com/buildset/buildset/content/repotest"
	"github.com/buildset/buildset/content/sqlite"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func TestRepository(t *testing.T) {
	repotest.Run(t, func(t *testing.T) content.Repository {
		t.Helper()

		db, err := sql.Open("sqlite", "file:"+t.TempDir()+"/test.db")
		require.NoError(t, err)

		db.SetMaxOpenConns(1)
		t.Cleanup(func() { db.Close() })

		repository, err := sqlite.NewRepository(t.Context(), db)
		require.NoError(t, err)

		return repository
	})
}
