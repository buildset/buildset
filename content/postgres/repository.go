// Package postgres stores content's posts in Postgres. It owns its schema and migrates itself, so
// wiring content to a different backend runs none of this.
//
// It is the twin of content/sqlite. The two are held to the same behaviour by the shared suite in
// content/repotest, and differ only in the placeholder style and the timestamp type.
package postgres

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"

	"github.com/Masterminds/squirrel"
	"github.com/buildset/buildset/content"
	"github.com/buildset/buildset/pkg/sqlmigrate"
)

//go:embed migrations/*.sql
var migrations embed.FS

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

type Repository struct {
	db *sql.DB
}

func NewRepository(ctx context.Context, db *sql.DB) (*Repository, error) {
	runner := sqlmigrate.Runner{
		FileSystem: migrations,
		Directory:  "migrations",
		TableName:  "content_schema_migrations",
		Dialect:    sqlmigrate.Postgres{},
	}

	if err := runner.Up(ctx, db); err != nil {
		return nil, fmt.Errorf("migrate content schema: %w", err)
	}

	return &Repository{db: db}, nil
}

// builder is the query builder bound to this backend. Naming the placeholder style once here is
// what keeps every statement below identical to its SQLite twin, including the filtered listing
// whose placeholders would otherwise have to be numbered by hand.
func (r *Repository) builder() squirrel.StatementBuilderType {
	return squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar).RunWith(r.db)
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

func (r *Repository) ListPosts(ctx context.Context, filter content.PostFilter) ([]content.Post, error) {
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
	defer rows.Close()

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

type rowScanner interface {
	Scan(dest ...any) error
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

	// A timestamptz comes back in the session's time zone. These are instants to everything above
	// this layer, so they are normalised rather than carrying the server's zone around.
	post.CreatedAt = post.CreatedAt.UTC()
	post.UpdatedAt = post.UpdatedAt.UTC()

	if publishedAt.Valid {
		published := publishedAt.Time.UTC()
		post.PublishedAt = &published
	}

	return &post, nil
}

func requireOneRow(result sql.Result, notFound error) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count affected rows: %w", err)
	}

	if affected == 0 {
		return notFound
	}

	return nil
}

var _ content.Repository = (*Repository)(nil)
