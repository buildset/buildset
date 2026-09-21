package postgres_test

import (
	"testing"

	"github.com/buildset/buildset/authz"
	"github.com/buildset/buildset/authz/postgres"
	"github.com/buildset/buildset/authz/repotest"
	"github.com/buildset/buildset/pkg/pgtest"
	"github.com/stretchr/testify/require"
)

func TestRepository(t *testing.T) {
	dsn := pgtest.DSN(t)

	repotest.Run(t, func(t *testing.T) authz.Repository {
		t.Helper()

		repository, err := postgres.NewRepository(t.Context(), pgtest.Open(t, dsn))
		require.NoError(t, err)

		return repository
	})
}
