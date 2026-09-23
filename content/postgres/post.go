package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Masterminds/squirrel"
	"github.com/buildset/buildset/content"
)

const tablePosts = "posts"

const (
	postColumnID          = "id"
	postColumnAuthorRef   = "author_ref"
	postColumnTitle       = "title"
	postColumnBody        = "body"
	postColumnContentType = "content_type"
	postColumnStatus      = "status"
	postColumnCreatedAt   = "created_at"
	postColumnUpdatedAt   = "updated_at"
	postColumnPublishedAt = "published_at"
)

func postColumns() []string {
	return []string{
		postColumnID,
		postColumnAuthorRef,
		postColumnTitle,
		postColumnBody,
		postColumnContentType,
		postColumnStatus,
		postColumnCreatedAt,
		postColumnUpdatedAt,
		postColumnPublishedAt,
	}
}

func (r *Repository) InsertPost(ctx context.Context, post *content.Post) error {
	_, err := r.builder().
		Insert(tablePosts).
		Columns(postColumns()...).
		Values(
			post.ID,
			post.AuthorRef,
			post.Title,
			post.Body,
			post.ContentType,
			string(post.Status),
			post.CreatedAt,
			post.UpdatedAt,
			post.PublishedAt,
		).
		ExecContext(ctx)
	if err != nil {
		return fmt.Errorf("insert post: %w", err)
	}

	return nil
}

// UpdatePost deliberately leaves author_ref alone: ownership is immutable.
func (r *Repository) UpdatePost(ctx context.Context, post *content.Post) error {
	result, err := r.builder().
		Update(tablePosts).
		Set(postColumnTitle, post.Title).
		Set(postColumnBody, post.Body).
		Set(postColumnContentType, post.ContentType).
		Set(postColumnStatus, string(post.Status)).
		Set(postColumnUpdatedAt, post.UpdatedAt).
		Set(postColumnPublishedAt, post.PublishedAt).
		Where(squirrel.Eq{postColumnID: post.ID}).
		ExecContext(ctx)
	if err != nil {
		return fmt.Errorf("update post: %w", err)
	}

	return requireOneRow(result, fmt.Errorf("%w: %s", content.ErrPostNotFound, post.ID))
}

func (r *Repository) DeletePost(ctx context.Context, id string) error {
	result, err := r.builder().
		Delete(tablePosts).
		Where(squirrel.Eq{postColumnID: id}).
		ExecContext(ctx)
	if err != nil {
		return fmt.Errorf("delete post: %w", err)
	}

	return requireOneRow(result, fmt.Errorf("%w: %s", content.ErrPostNotFound, id))
}

func (r *Repository) GetPost(ctx context.Context, id string) (*content.Post, error) {
	row := r.builder().
		Select(postColumns()...).
		From(tablePosts).
		Where(squirrel.Eq{postColumnID: id}).
		QueryRowContext(ctx)

	return scanPost(row, fmt.Errorf("%w: %s", content.ErrPostNotFound, id))
}

func (r *Repository) ListPosts(
	ctx context.Context,
	filter content.PostFilter,
) ([]content.Post, error) {
	query := r.builder().
		Select(postColumns()...).
		From(tablePosts)

	if filter.Status != "" {
		query = query.Where(squirrel.Eq{postColumnStatus: string(filter.Status)})
	}

	if filter.AuthorRef != "" {
		query = query.Where(squirrel.Eq{postColumnAuthorRef: filter.AuthorRef})
	}

	// Newest first, with the identifier as a tiebreak so the order is total and stable.
	rows, err := query.
		OrderBy(postColumnCreatedAt+" DESC", postColumnID+" DESC").
		Limit(uint64(filter.Limit)).
		QueryContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("select posts: %w", err)
	}
	defer func() { _ = rows.Close() }()

	posts := make([]content.Post, 0)

	for rows.Next() {
		post, err := scanPost(rows, content.ErrPostNotFound)
		if err != nil {
			return nil, err
		}

		posts = append(posts, *post)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate posts: %w", err)
	}

	return posts, nil
}

func scanPost(row rowScanner, notFound error) (*content.Post, error) {
	var (
		post        content.Post
		status      string
		publishedAt sql.NullTime
	)

	err := row.Scan(
		&post.ID, &post.AuthorRef, &post.Title, &post.Body, &post.ContentType,
		&status, &post.CreatedAt, &post.UpdatedAt, &publishedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, notFound
		}

		return nil, fmt.Errorf("scan post: %w", err)
	}

	post.Status = content.Status(status)

	// A timestamptz comes back in the session's time zone; everything above this layer wants an
	// instant.
	post.CreatedAt = post.CreatedAt.UTC()
	post.UpdatedAt = post.UpdatedAt.UTC()

	if publishedAt.Valid {
		published := publishedAt.Time.UTC()
		post.PublishedAt = &published
	}

	return &post, nil
}
