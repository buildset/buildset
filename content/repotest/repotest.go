// Package repotest is the contract every content.Repository must satisfy. Both backends run it, so
// a behaviour that differs between SQLite and Postgres fails here rather than in production.
package repotest

import (
	"context"
	"testing"
	"time"
	"uuid"

	"github.com/buildset/buildset/content"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// New builds a repository over empty storage.
type New func(t *testing.T) content.Repository

const (
	alice = "urn:auth:user:alice"
	bob   = "urn:auth:user:bob"
)

// Run exercises the whole contract.
func Run(t *testing.T, newRepository New) {
	t.Helper()

	t.Run("GetPost reports a missing post", func(t *testing.T) {
		repository := newRepository(t)

		_, err := repository.GetPost(context.Background(), uuid.NewV7().String())
		require.ErrorIs(t, err, content.ErrPostNotFound)
	})

	t.Run("UpdatePost reports a missing post", func(t *testing.T) {
		repository := newRepository(t)

		err := repository.UpdatePost(context.Background(), post(alice, "Ghost", content.StatusDraft))
		require.ErrorIs(t, err, content.ErrPostNotFound)
	})

	t.Run("DeletePost reports a missing post", func(t *testing.T) {
		repository := newRepository(t)

		err := repository.DeletePost(context.Background(), uuid.NewV7().String())
		require.ErrorIs(t, err, content.ErrPostNotFound)
	})

	t.Run("round-trips a draft", func(t *testing.T) {
		repository := newRepository(t)

		draft := post(alice, "First", content.StatusDraft)
		draft.Body = "Hello, world."
		require.NoError(t, repository.InsertPost(context.Background(), draft))

		stored, err := repository.GetPost(context.Background(), draft.ID)
		require.NoError(t, err)

		assert.Equal(t, draft.Title, stored.Title)
		assert.Equal(t, draft.Body, stored.Body)
		assert.Equal(t, alice, stored.AuthorRef)
		assert.Equal(t, content.StatusDraft, stored.Status)
		assert.Equal(t, content.ContentTypePlainText, stored.ContentType)
		// A draft has never been published, and the column is nullable on both backends.
		assert.Nil(t, stored.PublishedAt)
		assert.True(t, draft.CreatedAt.Truncate(time.Millisecond).Equal(stored.CreatedAt))
	})

	t.Run("round-trips a published post with its timestamp", func(t *testing.T) {
		repository := newRepository(t)

		published := post(alice, "Live", content.StatusPublished)
		at := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
		published.PublishedAt = &at
		require.NoError(t, repository.InsertPost(context.Background(), published))

		stored, err := repository.GetPost(context.Background(), published.ID)
		require.NoError(t, err)

		require.NotNil(t, stored.PublishedAt)
		assert.True(t, at.Equal(*stored.PublishedAt), "want %s, got %s", at, *stored.PublishedAt)
	})

	t.Run("UpdatePost leaves the author alone", func(t *testing.T) {
		repository := newRepository(t)

		draft := post(alice, "First", content.StatusDraft)
		require.NoError(t, repository.InsertPost(context.Background(), draft))

		// Ownership is immutable, so a repository asked to move a post to another author must not.
		draft.AuthorRef = bob
		draft.Title = "Renamed"
		require.NoError(t, repository.UpdatePost(context.Background(), draft))

		stored, err := repository.GetPost(context.Background(), draft.ID)
		require.NoError(t, err)
		assert.Equal(t, "Renamed", stored.Title)
		assert.Equal(t, alice, stored.AuthorRef)
	})

	t.Run("ListPosts filters and orders", func(t *testing.T) {
		repository := newRepository(t)

		base := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

		// Two posts share a timestamp so the id tiebreak decides, with ids that differ by
		// punctuation, which is where a locale collation would disagree with byte order.
		first := post(alice, "Oldest", content.StatusDraft)
		first.ID, first.CreatedAt = "aa-bb", base

		second := post(alice, "Same time", content.StatusDraft)
		second.ID, second.CreatedAt = "aaXbb", base

		newest := post(bob, "Newest", content.StatusPublished)
		newest.CreatedAt = base.Add(time.Hour)
		at := newest.CreatedAt
		newest.PublishedAt = &at

		for _, p := range []*content.Post{first, second, newest} {
			require.NoError(t, repository.InsertPost(context.Background(), p))
		}

		t.Run("unfiltered, newest first with the id as tiebreak", func(t *testing.T) {
			posts, err := repository.ListPosts(context.Background(), content.PostFilter{Limit: 10})
			require.NoError(t, err)
			require.Len(t, posts, 3)
			// "X" is 0x58 and "-" is 0x2D, so DESC byte order puts aaXbb before aa-bb.
			assert.Equal(t, []string{newest.ID, "aaXbb", "aa-bb"},
				[]string{posts[0].ID, posts[1].ID, posts[2].ID})
		})

		t.Run("by status", func(t *testing.T) {
			posts, err := repository.ListPosts(context.Background(), content.PostFilter{
				Status: content.StatusPublished,
				Limit:  10,
			})
			require.NoError(t, err)
			require.Len(t, posts, 1)
			assert.Equal(t, newest.ID, posts[0].ID)
		})

		t.Run("by author", func(t *testing.T) {
			posts, err := repository.ListPosts(context.Background(), content.PostFilter{
				AuthorRef: alice,
				Limit:     10,
			})
			require.NoError(t, err)
			assert.Len(t, posts, 2)
		})

		t.Run("by status and author together", func(t *testing.T) {
			posts, err := repository.ListPosts(context.Background(), content.PostFilter{
				Status:    content.StatusDraft,
				AuthorRef: alice,
				Limit:     10,
			})
			require.NoError(t, err)
			assert.Len(t, posts, 2)

			posts, err = repository.ListPosts(context.Background(), content.PostFilter{
				Status:    content.StatusPublished,
				AuthorRef: alice,
				Limit:     10,
			})
			require.NoError(t, err)
			assert.Empty(t, posts)
		})

		t.Run("honours the limit", func(t *testing.T) {
			posts, err := repository.ListPosts(context.Background(), content.PostFilter{Limit: 1})
			require.NoError(t, err)
			require.Len(t, posts, 1)
			assert.Equal(t, newest.ID, posts[0].ID)
		})

		t.Run("matches nothing for an unknown author", func(t *testing.T) {
			posts, err := repository.ListPosts(context.Background(), content.PostFilter{
				AuthorRef: "urn:auth:user:nobody",
				Limit:     10,
			})
			require.NoError(t, err)
			assert.Empty(t, posts)
		})
	})

	t.Run("DeletePost removes it", func(t *testing.T) {
		repository := newRepository(t)

		draft := post(alice, "Doomed", content.StatusDraft)
		require.NoError(t, repository.InsertPost(context.Background(), draft))
		require.NoError(t, repository.DeletePost(context.Background(), draft.ID))

		_, err := repository.GetPost(context.Background(), draft.ID)
		require.ErrorIs(t, err, content.ErrPostNotFound)
	})
}

func post(authorRef, title string, status content.Status) *content.Post {
	now := time.Now().UTC().Truncate(time.Millisecond)

	return &content.Post{
		ID:          uuid.NewV7().String(),
		AuthorRef:   authorRef,
		Title:       title,
		ContentType: content.ContentTypePlainText,
		Status:      status,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}
