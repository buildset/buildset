package hash_test

import (
	"strings"
	"testing"

	"github.com/buildset/buildset/auth/hash"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// legacy stands in for an algorithm that is registered but no longer preferred, which is what
// argon2id will make bcrypt one day.
type legacy struct{}

func (legacy) Name() string          { return "legacy" }
func (legacy) Identifiers() []string { return []string{"legacy"} }

func (legacy) Hash(password string) (string, error) { return "$legacy$" + password, nil }

func (legacy) Verify(encoded, password string) error {
	if encoded != "$legacy$"+password {
		return hash.ErrMismatch
	}

	return nil
}

func newRegistry(t *testing.T) *hash.Registry {
	t.Helper()

	// Minimum cost keeps the test fast; production cost comes from AUTH_BCRYPT_COST.
	bcryptAlgorithm, err := hash.NewBcrypt(4)
	require.NoError(t, err)

	registry, err := hash.NewRegistry(bcryptAlgorithm, legacy{})
	require.NoError(t, err)

	return registry
}

func TestRegistryHashAndVerify(t *testing.T) {
	t.Parallel()

	registry := newRegistry(t)

	encoded, err := registry.Hash("correct horse battery")
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(encoded, "$2a$"), "bcrypt hashes carry their identifier: %s", encoded)

	require.NoError(t, registry.Verify(encoded, "correct horse battery"))
	require.ErrorIs(t, registry.Verify(encoded, "wrong horse battery"), hash.ErrMismatch)
}

func TestRegistryDispatchesOnIdentifier(t *testing.T) {
	t.Parallel()

	registry := newRegistry(t)

	require.NoError(t, registry.Verify("$legacy$hunter2hunter2", "hunter2hunter2"))
	require.ErrorIs(t, registry.Verify("$legacy$hunter2hunter2", "something else"), hash.ErrMismatch)

	// The seam that lets argon2id arrive later: an unregistered identifier is reported, not guessed at.
	require.ErrorIs(t, registry.Verify("$argon2id$v=19$m=65536,t=3,p=4$c2FsdA$aGFzaA", "whatever"), hash.ErrUnknownAlgorithm)
	require.ErrorIs(t, registry.Verify("not-a-hash", "whatever"), hash.ErrUnknownAlgorithm)
}

func TestRegistryNeedsRehash(t *testing.T) {
	t.Parallel()

	registry := newRegistry(t)

	encoded, err := registry.Hash("correct horse battery")
	require.NoError(t, err)
	assert.False(t, registry.NeedsRehash(encoded))

	assert.True(t, registry.NeedsRehash("$legacy$correct horse battery"))
	assert.True(t, registry.NeedsRehash("$argon2id$v=19$m=65536,t=3,p=4$c2FsdA$aGFzaA"))
}

func TestRegistryRejectsBadPasswordLengths(t *testing.T) {
	t.Parallel()

	registry := newRegistry(t)

	_, err := registry.Hash(strings.Repeat("a", hash.MinPasswordLength-1))
	require.ErrorIs(t, err, hash.ErrPasswordTooShort)

	// bcrypt truncates past 72 bytes, which would let a shorter password unlock the account.
	_, err = registry.Hash(strings.Repeat("a", hash.MaxBcryptPasswordLength+1))
	require.ErrorIs(t, err, hash.ErrPasswordTooLong)
}
