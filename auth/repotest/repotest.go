// Package repotest is the contract every auth.Repository must satisfy. Both backends run it, so a
// behaviour that differs between SQLite and Postgres fails here rather than in production.
package repotest

import (
	"context"
	"fmt"
	"testing"
	"time"
	"uuid"

	"github.com/buildset/buildset/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// New builds a repository over empty storage, once per subtest, so no test sees another's rows.
type New func(t *testing.T) auth.Repository

// Run exercises the whole contract.
func Run(t *testing.T, newRepository New) {
	t.Helper()

	t.Run("InsertUser rejects a duplicate username", func(t *testing.T) {
		repository := newRepository(t)

		require.NoError(t, repository.InsertUser(context.Background(), user("alice")))

		duplicate := user("alice")
		duplicate.ID = uuid.NewV7().String()

		err := repository.InsertUser(context.Background(), duplicate)
		require.ErrorIs(t, err, auth.ErrUsernameTaken)
	})

	t.Run("UpdateUser rejects a username another account holds", func(t *testing.T) {
		repository := newRepository(t)

		alice := user("alice")
		bob := user("bob")
		require.NoError(t, repository.InsertUser(context.Background(), alice))
		require.NoError(t, repository.InsertUser(context.Background(), bob))

		bob.Username = "alice"

		err := repository.UpdateUser(context.Background(), bob)
		require.ErrorIs(t, err, auth.ErrUsernameTaken)
	})

	t.Run("UpdateUser reports a missing account", func(t *testing.T) {
		repository := newRepository(t)

		err := repository.UpdateUser(context.Background(), user("ghost"))
		require.ErrorIs(t, err, auth.ErrUserNotFound)
	})

	t.Run("DeleteUser reports a missing account", func(t *testing.T) {
		repository := newRepository(t)

		err := repository.DeleteUser(context.Background(), uuid.NewV7().String())
		require.ErrorIs(t, err, auth.ErrUserNotFound)
	})

	t.Run("GetUser reports a missing account", func(t *testing.T) {
		repository := newRepository(t)

		_, err := repository.GetUser(context.Background(), uuid.NewV7().String())
		require.ErrorIs(t, err, auth.ErrUserNotFound)
	})

	t.Run("GetUserByUsername reports a missing account", func(t *testing.T) {
		repository := newRepository(t)

		_, err := repository.GetUserByUsername(context.Background(), "nobody")
		require.ErrorIs(t, err, auth.ErrUserNotFound)
	})

	t.Run("round-trips a user to the millisecond", func(t *testing.T) {
		repository := newRepository(t)

		alice := user("alice")
		alice.Name = "Alice Example"
		require.NoError(t, repository.InsertUser(context.Background(), alice))

		stored, err := repository.GetUser(context.Background(), alice.ID)
		require.NoError(t, err)

		assert.Equal(t, alice.Username, stored.Username)
		assert.Equal(t, alice.Name, stored.Name)
		assert.Equal(t, alice.PasswordHash, stored.PasswordHash)
		// The stored format carries milliseconds and nothing finer, on both backends.
		assert.True(t, alice.CreatedAt.Truncate(time.Millisecond).Equal(stored.CreatedAt),
			"created_at %s became %s", alice.CreatedAt, stored.CreatedAt)
		assert.Equal(t, time.UTC, stored.CreatedAt.Location())
	})

	t.Run("ListUsers orders by created_at then id", func(t *testing.T) {
		repository := newRepository(t)

		// Two users share a timestamp so the id tiebreak decides, and the ids are chosen to differ
		// by punctuation, which is where a locale collation would disagree with byte order.
		base := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

		first := user("carol")
		first.ID, first.CreatedAt = "aa-bb", base

		second := user("dave")
		second.ID, second.CreatedAt = "aaXbb", base

		third := user("erin")
		third.CreatedAt = base.Add(time.Second)

		for _, u := range []*auth.User{third, second, first} {
			require.NoError(t, repository.InsertUser(context.Background(), u))
		}

		users, err := repository.ListUsers(context.Background(), 10)
		require.NoError(t, err)
		require.Len(t, users, 3)

		// "-" is 0x2D and "X" is 0x58, so byte order puts aa-bb first.
		assert.Equal(t, []string{"aa-bb", "aaXbb", third.ID}, []string{users[0].ID, users[1].ID, users[2].ID})
	})

	t.Run("ListUsers honours the limit", func(t *testing.T) {
		repository := newRepository(t)

		for i := range 5 {
			require.NoError(t, repository.InsertUser(context.Background(), user(fmt.Sprintf("user%d", i))))
		}

		users, err := repository.ListUsers(context.Background(), 2)
		require.NoError(t, err)
		assert.Len(t, users, 2)
	})

	t.Run("CountUsers counts", func(t *testing.T) {
		repository := newRepository(t)

		count, err := repository.CountUsers(context.Background())
		require.NoError(t, err)
		assert.Equal(t, 0, count)

		require.NoError(t, repository.InsertUser(context.Background(), user("alice")))

		count, err = repository.CountUsers(context.Background())
		require.NoError(t, err)
		assert.Equal(t, 1, count)
	})

	t.Run("DeleteUser cascades to sessions", func(t *testing.T) {
		repository := newRepository(t)

		alice := user("alice")
		require.NoError(t, repository.InsertUser(context.Background(), alice))

		session := session(alice.ID, "hash-1", time.Now().Add(time.Hour))
		require.NoError(t, repository.InsertSession(context.Background(), session))

		require.NoError(t, repository.DeleteUser(context.Background(), alice.ID))

		_, err := repository.GetSessionByTokenHash(context.Background(), "hash-1")
		require.ErrorIs(t, err, auth.ErrSessionNotFound)
	})

	t.Run("GetSessionByTokenHash reports a missing session", func(t *testing.T) {
		repository := newRepository(t)

		_, err := repository.GetSessionByTokenHash(context.Background(), "nothing")
		require.ErrorIs(t, err, auth.ErrSessionNotFound)
	})

	t.Run("round-trips a session", func(t *testing.T) {
		repository := newRepository(t)

		alice := user("alice")
		require.NoError(t, repository.InsertUser(context.Background(), alice))

		expires := time.Now().Add(time.Hour)
		original := session(alice.ID, "hash-1", expires)
		original.UserAgent = "probe/1.0"
		original.IP = "203.0.113.7"
		require.NoError(t, repository.InsertSession(context.Background(), original))

		stored, err := repository.GetSessionByTokenHash(context.Background(), "hash-1")
		require.NoError(t, err)

		assert.Equal(t, original.ID, stored.ID)
		assert.Equal(t, alice.ID, stored.UserID)
		assert.Equal(t, "probe/1.0", stored.UserAgent)
		assert.Equal(t, "203.0.113.7", stored.IP)
		assert.True(t, expires.UTC().Truncate(time.Millisecond).Equal(stored.ExpiresAt))
	})

	t.Run("TouchSession moves last_seen", func(t *testing.T) {
		repository := newRepository(t)

		alice := user("alice")
		require.NoError(t, repository.InsertUser(context.Background(), alice))
		require.NoError(t, repository.InsertSession(context.Background(), session(alice.ID, "hash-1", time.Now().Add(time.Hour))))

		later := time.Now().Add(time.Minute).UTC().Truncate(time.Millisecond)
		require.NoError(t, repository.TouchSession(context.Background(), "session-hash-1", later))

		stored, err := repository.GetSessionByTokenHash(context.Background(), "hash-1")
		require.NoError(t, err)
		assert.True(t, later.Equal(stored.LastSeen), "want %s, got %s", later, stored.LastSeen)
	})

	t.Run("DeleteSessionsByUser keeps the exception", func(t *testing.T) {
		repository := newRepository(t)

		alice := user("alice")
		require.NoError(t, repository.InsertUser(context.Background(), alice))

		for _, hash := range []string{"hash-1", "hash-2", "hash-3"} {
			require.NoError(t, repository.InsertSession(context.Background(), session(alice.ID, hash, time.Now().Add(time.Hour))))
		}

		require.NoError(t, repository.DeleteSessionsByUser(context.Background(), alice.ID, "session-hash-2"))

		_, err := repository.GetSessionByTokenHash(context.Background(), "hash-2")
		require.NoError(t, err)

		for _, gone := range []string{"hash-1", "hash-3"} {
			_, err := repository.GetSessionByTokenHash(context.Background(), gone)
			require.ErrorIs(t, err, auth.ErrSessionNotFound)
		}
	})

	t.Run("DeleteSessionsByUser with no exception removes all", func(t *testing.T) {
		repository := newRepository(t)

		alice := user("alice")
		require.NoError(t, repository.InsertUser(context.Background(), alice))
		require.NoError(t, repository.InsertSession(context.Background(), session(alice.ID, "hash-1", time.Now().Add(time.Hour))))

		require.NoError(t, repository.DeleteSessionsByUser(context.Background(), alice.ID, ""))

		_, err := repository.GetSessionByTokenHash(context.Background(), "hash-1")
		require.ErrorIs(t, err, auth.ErrSessionNotFound)
	})

	t.Run("DeleteExpiredSessions is inclusive at the boundary", func(t *testing.T) {
		repository := newRepository(t)

		alice := user("alice")
		require.NoError(t, repository.InsertUser(context.Background(), alice))

		now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

		require.NoError(t, repository.InsertSession(context.Background(), session(alice.ID, "past", now.Add(-time.Hour))))
		require.NoError(t, repository.InsertSession(context.Background(), session(alice.ID, "exact", now)))
		require.NoError(t, repository.InsertSession(context.Background(), session(alice.ID, "future", now.Add(time.Hour))))

		// The comparison is <=, so a session expiring exactly now goes.
		deleted, err := repository.DeleteExpiredSessions(context.Background(), now)
		require.NoError(t, err)
		assert.Equal(t, int64(2), deleted)

		_, err = repository.GetSessionByTokenHash(context.Background(), "future")
		require.NoError(t, err)
	})

	t.Run("DeleteSessionByTokenHash is quiet about a missing session", func(t *testing.T) {
		repository := newRepository(t)

		require.NoError(t, repository.DeleteSessionByTokenHash(context.Background(), "nothing"))
	})
}

func user(username string) *auth.User {
	now := time.Now().UTC().Truncate(time.Millisecond)

	return &auth.User{
		ID:           uuid.NewV7().String(),
		Username:     username,
		PasswordHash: "$2a$10$" + username,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

func session(userID, tokenHash string, expires time.Time) *auth.Session {
	now := time.Now().UTC().Truncate(time.Millisecond)

	return &auth.Session{
		ID:        "session-" + tokenHash,
		UserID:    userID,
		TokenHash: tokenHash,
		CreatedAt: now,
		ExpiresAt: expires.UTC().Truncate(time.Millisecond),
		LastSeen:  now,
	}
}
