// Package postgres stores content's posts in Postgres. It owns its schema and migrates itself, so
// wiring content to a different backend runs none of this.
//
// It is the twin of content/sqlite and deliberately mirrors it statement for statement: the two
// are held to the same behaviour by the shared suite in content/repotest.
package postgres

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/buildset/buildset/content"
	"github.com/buildset/buildset/pkg/sqlmigrate"
)

//go:embed migrations/*.sql
var migrations embed.FS

// timeFormat sorts lexicographically, so ordering and range queries work on the stored text. The
// columns holding it are COLLATE "C", which is what makes that true whatever the database locale.
//
// TODO: move to timestamptz. Every conversion funnels through formatTime and parseTime, so it is
// an ALTER per column plus the scan targets in this file.
const timeFormat = "2006-01-02T15:04:05.000Z"

const postColumns = `id, author_ref, title, body, content_type, status, created_at, updated_at, published_at`

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

func (r *Repository) InsertPost(ctx context.Context, post *content.Post) error {
	const query = `INSERT INTO posts (` + postColumns + `) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`

	_, err := r.db.ExecContext(ctx, query,
		post.ID, post.AuthorRef, post.Title, post.Body, post.ContentType, string(post.Status),
		formatTime(post.CreatedAt), formatTime(post.UpdatedAt), formatOptionalTime(post.PublishedAt),
	)
	if err != nil {
		return fmt.Errorf("insert post: %w", err)
	}

	return nil
}

func (r *Repository) UpdatePost(ctx context.Context, post *content.Post) error {
	const query = `UPDATE posts
		SET title = $1, body = $2, content_type = $3, status = $4, updated_at = $5, published_at = $6
		WHERE id = $7`

	result, err := r.db.ExecContext(ctx, query,
		post.Title, post.Body, post.ContentType, string(post.Status),
		formatTime(post.UpdatedAt), formatOptionalTime(post.PublishedAt), post.ID,
	)
	if err != nil {
		return fmt.Errorf("update post: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count affected rows: %w", err)
	}

	if affected == 0 {
		return content.ErrPostNotFound
	}

	return nil
}

func (r *Repository) DeletePost(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM posts WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete post: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count affected rows: %w", err)
	}

	if affected == 0 {
		return content.ErrPostNotFound
	}

	return nil
}

func (r *Repository) GetPost(ctx context.Context, id string) (*content.Post, error) {
	const query = `SELECT ` + postColumns + ` FROM posts WHERE id = $1`

	return scanPost(r.db.QueryRowContext(ctx, query, id))
}

func (r *Repository) ListPosts(ctx context.Context, filter content.PostFilter) ([]content.Post, error) {
	query := `SELECT ` + postColumns + ` FROM posts`

	var (
		conditions []string
		arguments  []any
	)

	// Each condition appends its argument first and numbers its placeholder from the resulting
	// length, so adding or skipping a condition never renumbers the others.
	if filter.Status != "" {
		arguments = append(arguments, string(filter.Status))
		conditions = append(conditions, fmt.Sprintf(`status = $%d`, len(arguments)))
	}

	if filter.AuthorRef != "" {
		arguments = append(arguments, filter.AuthorRef)
		conditions = append(conditions, fmt.Sprintf(`author_ref = $%d`, len(arguments)))
	}

	if len(conditions) > 0 {
		query += ` WHERE ` + strings.Join(conditions, ` AND `)
	}

	// Newest first, with the identifier as a tiebreak so the order is total and stable.
	arguments = append(arguments, filter.Limit)
	query += fmt.Sprintf(` ORDER BY created_at DESC, id DESC LIMIT $%d`, len(arguments))

	rows, err := r.db.QueryContext(ctx, query, arguments...)
	if err != nil {
		return nil, fmt.Errorf("select posts: %w", err)
	}
	defer rows.Close()

	var posts []content.Post

	for rows.Next() {
		post, err := scanPost(rows)
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

func scanPost(row rowScanner) (*content.Post, error) {
	var (
		post                 content.Post
		status               string
		createdAt, updatedAt string
		publishedAt          sql.NullString
	)

	err := row.Scan(
		&post.ID, &post.AuthorRef, &post.Title, &post.Body, &post.ContentType,
		&status, &createdAt, &updatedAt, &publishedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, content.ErrPostNotFound
		}

		return nil, fmt.Errorf("scan post: %w", err)
	}

	post.Status = content.Status(status)

	if post.CreatedAt, err = parseTime(createdAt); err != nil {
		return nil, fmt.Errorf("parse post created_at: %w", err)
	}

	if post.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return nil, fmt.Errorf("parse post updated_at: %w", err)
	}

	if publishedAt.Valid {
		parsed, err := parseTime(publishedAt.String)
		if err != nil {
			return nil, fmt.Errorf("parse post published_at: %w", err)
		}

		post.PublishedAt = &parsed
	}

	return &post, nil
}

func formatTime(t time.Time) string {
	return t.UTC().Format(timeFormat)
}

func formatOptionalTime(t *time.Time) any {
	if t == nil {
		return nil
	}

	return formatTime(*t)
}

func parseTime(value string) (time.Time, error) {
	return time.Parse(timeFormat, value)
}

var _ content.Repository = (*Repository)(nil)
