// Package repotest is the contract every authz.Repository must satisfy. Both backends run it, so a
// behaviour that differs between SQLite and Postgres fails here rather than in production.
package repotest

import (
	"context"
	"testing"
	"time"

	"github.com/buildset/buildset/authz"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// New builds a repository over empty storage, already carrying the seeded roles.
type New func(t *testing.T) authz.Repository

const (
	alice = "urn:auth:user:alice"
	post  = "urn:content:post:1"
)

// Run exercises the whole contract.
func Run(t *testing.T, newRepository New) {
	t.Helper()

	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

	t.Run("ListRoles returns the seeded roles", func(t *testing.T) {
		repository := newRepository(t)

		roles, err := repository.ListRoles(context.Background())
		require.NoError(t, err)

		names := make([]string, 0, len(roles))
		for _, role := range roles {
			names = append(names, role.Name)
		}

		assert.ElementsMatch(t, []string{"admin", "author", "reader"}, names)
	})

	t.Run("RoleExists distinguishes seeded from unknown", func(t *testing.T) {
		repository := newRepository(t)

		exists, err := repository.RoleExists(context.Background(), "admin")
		require.NoError(t, err)
		assert.True(t, exists)

		exists, err = repository.RoleExists(context.Background(), "wizard")
		require.NoError(t, err)
		assert.False(t, exists)
	})

	t.Run("InsertSubjectRole rejects an unknown role", func(t *testing.T) {
		repository := newRepository(t)

		err := repository.InsertSubjectRole(context.Background(), alice, "wizard", now)
		require.ErrorIs(t, err, authz.ErrUnknownRole)
	})

	t.Run("InsertSubjectRole is idempotent", func(t *testing.T) {
		repository := newRepository(t)

		require.NoError(t, repository.InsertSubjectRole(context.Background(), alice, "admin", now))
		require.NoError(
			t,
			repository.InsertSubjectRole(context.Background(), alice, "admin", now.Add(time.Hour)),
		)

		roles, err := repository.SubjectRoles(context.Background(), alice)
		require.NoError(t, err)
		assert.Equal(t, []string{"admin"}, roles)
	})

	t.Run("SubjectRoles is empty for an unknown subject", func(t *testing.T) {
		repository := newRepository(t)

		roles, err := repository.SubjectRoles(context.Background(), "urn:auth:user:nobody")
		require.NoError(t, err)
		assert.Empty(t, roles)
	})

	t.Run("DeleteSubjectRole removes one role", func(t *testing.T) {
		repository := newRepository(t)

		require.NoError(t, repository.InsertSubjectRole(context.Background(), alice, "admin", now))
		require.NoError(t, repository.InsertSubjectRole(context.Background(), alice, "author", now))
		require.NoError(t, repository.DeleteSubjectRole(context.Background(), alice, "admin"))

		roles, err := repository.SubjectRoles(context.Background(), alice)
		require.NoError(t, err)
		assert.Equal(t, []string{"author"}, roles)
	})

	t.Run("SubjectPatterns unions role permissions and direct grants", func(t *testing.T) {
		repository := newRepository(t)

		require.NoError(t, repository.InsertSubjectRole(context.Background(), alice, "author", now))
		require.NoError(
			t,
			repository.InsertGrants(
				context.Background(),
				alice,
				[]string{"post.update", "post.delete"},
				post,
				now,
			),
		)

		patterns, err := repository.SubjectPatterns(context.Background(), alice)
		require.NoError(t, err)

		assert.ElementsMatch(t, []authz.Pattern{
			{Action: "post.create", Resource: "urn:content:post:*"},
			{Action: "post.update", Resource: post},
			{Action: "post.delete", Resource: post},
		}, patterns)
	})

	t.Run("SubjectPatterns is empty for an unknown subject", func(t *testing.T) {
		repository := newRepository(t)

		patterns, err := repository.SubjectPatterns(context.Background(), "urn:auth:user:nobody")
		require.NoError(t, err)
		assert.Empty(t, patterns)
	})

	t.Run("InsertGrants is idempotent", func(t *testing.T) {
		repository := newRepository(t)

		require.NoError(
			t,
			repository.InsertGrants(
				context.Background(),
				alice,
				[]string{"post.update"},
				post,
				now,
			),
		)
		require.NoError(
			t,
			repository.InsertGrants(
				context.Background(),
				alice,
				[]string{"post.update"},
				post,
				now.Add(time.Hour),
			),
		)

		patterns, err := repository.SubjectPatterns(context.Background(), alice)
		require.NoError(t, err)
		assert.Len(t, patterns, 1)
	})

	t.Run("InsertGrants with no actions writes nothing", func(t *testing.T) {
		repository := newRepository(t)

		require.NoError(t, repository.InsertGrants(context.Background(), alice, nil, post, now))

		patterns, err := repository.SubjectPatterns(context.Background(), alice)
		require.NoError(t, err)
		assert.Empty(t, patterns)
	})

	t.Run("DeleteBySubject removes roles and grants together", func(t *testing.T) {
		repository := newRepository(t)

		require.NoError(t, repository.InsertSubjectRole(context.Background(), alice, "author", now))
		require.NoError(
			t,
			repository.InsertGrants(
				context.Background(),
				alice,
				[]string{"post.update"},
				post,
				now,
			),
		)

		require.NoError(t, repository.DeleteBySubject(context.Background(), alice))

		roles, err := repository.SubjectRoles(context.Background(), alice)
		require.NoError(t, err)
		assert.Empty(t, roles)

		patterns, err := repository.SubjectPatterns(context.Background(), alice)
		require.NoError(t, err)
		assert.Empty(t, patterns)
	})

	t.Run("DeleteByResource removes grants and leaves roles", func(t *testing.T) {
		repository := newRepository(t)

		require.NoError(t, repository.InsertSubjectRole(context.Background(), alice, "author", now))
		require.NoError(
			t,
			repository.InsertGrants(
				context.Background(),
				alice,
				[]string{"post.update"},
				post,
				now,
			),
		)

		require.NoError(t, repository.DeleteByResource(context.Background(), post))

		roles, err := repository.SubjectRoles(context.Background(), alice)
		require.NoError(t, err)
		assert.Equal(t, []string{"author"}, roles)

		patterns, err := repository.SubjectPatterns(context.Background(), alice)
		require.NoError(t, err)
		assert.Equal(
			t,
			[]authz.Pattern{{Action: "post.create", Resource: "urn:content:post:*"}},
			patterns,
		)
	})

	t.Run("deleting an absent subject or resource is quiet", func(t *testing.T) {
		repository := newRepository(t)

		require.NoError(t, repository.DeleteBySubject(context.Background(), "urn:auth:user:nobody"))
		require.NoError(
			t,
			repository.DeleteByResource(context.Background(), "urn:content:post:nothing"),
		)
		require.NoError(t, repository.DeleteSubjectRole(context.Background(), alice, "admin"))
	})
}
