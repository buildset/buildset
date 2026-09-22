package authz_test

import (
	"database/sql"
	"testing"

	"github.com/buildset/buildset/authz"
	authzsqlite "github.com/buildset/buildset/authz/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

const (
	adminRef  = "urn:auth:user:0199bf3c-7a1e-7c2b-9f10-000000000001"
	authorRef = "urn:auth:user:0199bf3c-7a1e-7c2b-9f10-000000000002"
	readerRef = "urn:auth:user:0199bf3c-7a1e-7c2b-9f10-000000000003"
	postRef   = "urn:content:post:0199bf3c-7a1e-7c2b-9f10-0000000000aa"
	otherPost = "urn:content:post:0199bf3c-7a1e-7c2b-9f10-0000000000bb"
)

func newService(t *testing.T) *authz.Service {
	t.Helper()

	db, err := sql.Open("sqlite", "file:"+t.Name()+"?mode=memory&cache=shared&_pragma=foreign_keys(ON)")
	require.NoError(t, err)

	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	repository, err := authzsqlite.NewRepository(t.Context(), db)
	require.NoError(t, err)

	return authz.NewService(repository)
}

func TestSeededRoles(t *testing.T) {
	service := newService(t)

	roles, err := service.ListRoles(t.Context())
	require.NoError(t, err)

	names := make([]string, 0, len(roles))
	for _, role := range roles {
		names = append(names, role.Name)
	}

	assert.Equal(t, []string{"admin", "author", "reader"}, names)
}

func TestAdminRoleAllowsEverything(t *testing.T) {
	service := newService(t)
	ctx := t.Context()

	require.NoError(t, service.AssignRole(ctx, adminRef, "admin"))

	for _, check := range []struct{ action, resource string }{
		{"post.delete", postRef},
		{"user.read", readerRef},
		{"role.assign", authorRef},
	} {
		allowed, err := service.Can(ctx, adminRef, check.action, check.resource)
		require.NoError(t, err)
		assert.True(t, allowed, "%s on %s", check.action, check.resource)
	}
}

func TestAuthorRoleAllowsCreationOnly(t *testing.T) {
	service := newService(t)
	ctx := t.Context()

	require.NoError(t, service.AssignRole(ctx, authorRef, "author"))

	allowed, err := service.Can(ctx, authorRef, "post.create", postRef)
	require.NoError(t, err)
	assert.True(t, allowed)

	// Creating a post is not the same as being allowed to change any post.
	allowed, err = service.Can(ctx, authorRef, "post.update", postRef)
	require.NoError(t, err)
	assert.False(t, allowed)

	// The author role's pattern is scoped to posts and must not reach another service's resources.
	allowed, err = service.Can(ctx, authorRef, "post.create", readerRef)
	require.NoError(t, err)
	assert.False(t, allowed)
}

func TestReaderRoleGrantsNothing(t *testing.T) {
	service := newService(t)
	ctx := t.Context()

	require.NoError(t, service.AssignRole(ctx, readerRef, "reader"))

	allowed, err := service.Can(ctx, readerRef, "post.read", postRef)
	require.NoError(t, err)
	assert.False(t, allowed)
}

// A direct grant is how a service expresses ownership of one resource without authz knowing what
// that resource is.
func TestGrantAppliesToExactlyOneResource(t *testing.T) {
	service := newService(t)
	ctx := t.Context()

	require.NoError(t, service.Grant(ctx, authorRef, []string{"post.update", "post.delete"}, postRef))

	for _, action := range []string{"post.update", "post.delete"} {
		allowed, err := service.Can(ctx, authorRef, action, postRef)
		require.NoError(t, err)
		assert.True(t, allowed, action)
	}

	allowed, err := service.Can(ctx, authorRef, "post.publish", postRef)
	require.NoError(t, err)
	assert.False(t, allowed, "an action that was not granted")

	allowed, err = service.Can(ctx, authorRef, "post.update", otherPost)
	require.NoError(t, err)
	assert.False(t, allowed, "another resource")
}

func TestRevocationAndPurge(t *testing.T) {
	service := newService(t)
	ctx := t.Context()

	require.NoError(t, service.AssignRole(ctx, adminRef, "admin"))
	require.NoError(t, service.Grant(ctx, authorRef, []string{"post.update"}, postRef))

	require.NoError(t, service.RevokeRole(ctx, adminRef, "admin"))
	allowed, err := service.Can(ctx, adminRef, "post.delete", postRef)
	require.NoError(t, err)
	assert.False(t, allowed)

	// Deleting a post must not leave grants pointing at a reference that no longer resolves.
	require.NoError(t, service.PurgeResource(ctx, postRef))
	allowed, err = service.Can(ctx, authorRef, "post.update", postRef)
	require.NoError(t, err)
	assert.False(t, allowed)
}

func TestRejectsMalformedInput(t *testing.T) {
	service := newService(t)
	ctx := t.Context()

	_, err := service.Can(ctx, "not-a-reference", "post.read", postRef)
	require.Error(t, err)

	_, err = service.Can(ctx, adminRef, "post read", postRef)
	require.ErrorIs(t, err, authz.ErrInvalidAction)

	require.ErrorIs(t, service.AssignRole(ctx, adminRef, "superuser"), authz.ErrUnknownRole)
}
