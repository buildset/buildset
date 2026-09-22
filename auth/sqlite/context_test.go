package sqlite_test

import (
	"database/sql"
	"testing"

	"github.com/buildset/buildset/auth"
	authhttpapi "github.com/buildset/buildset/auth/httpapi"
	"github.com/buildset/buildset/auth/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

// The wrapped id must reach a log line without reaching the visitor.
func TestErrorCarriesContextButDoesNotLeakIt(t *testing.T) {
	db, err := sql.Open("sqlite", "file:"+t.TempDir()+"/t.db?_pragma=foreign_keys(ON)")
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	repository, err := sqlite.NewRepository(t.Context(), db)
	require.NoError(t, err)

	_, err = repository.GetUser(t.Context(), "01a0-missing-id")
	require.Error(t, err)

	// errors.Is still works, which every Classify and web/session.go depends on.
	assert.ErrorIs(t, err, auth.ErrUserNotFound)

	// The id is in the error, so a log line names the record that was missing.
	assert.Contains(t, err.Error(), "01a0-missing-id")

	// But the sentence shown to a visitor is the fixed one, with no id in it.
	code, message, ok := authhttpapi.Classify(err)
	require.True(t, ok)
	assert.Equal(t, "not_found", string(code))
	assert.Equal(t, "That account could not be found.", message)
	assert.NotContains(t, message, "01a0-missing-id")
}
