package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Masterminds/squirrel"
	"github.com/buildset/buildset/auth"
)

const tableUsers = "users"

const (
	userColumnID           = "id"
	userColumnUsername     = "username"
	userColumnName         = "name"
	userColumnPasswordHash = "password_hash"
	userColumnCreatedAt    = "created_at"
	userColumnUpdatedAt    = "updated_at"
)

func userColumns() []string {
	return []string{
		userColumnID,
		userColumnUsername,
		userColumnName,
		userColumnPasswordHash,
		userColumnCreatedAt,
		userColumnUpdatedAt,
	}
}

func (r *Repository) InsertUser(ctx context.Context, user *auth.User) error {
	_, err := r.builder().
		Insert(tableUsers).
		Columns(userColumns()...).
		Values(
			user.ID,
			user.Username,
			user.Name,
			user.PasswordHash,
			formatTime(user.CreatedAt),
			formatTime(user.UpdatedAt),
		).
		ExecContext(ctx)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("%w: %s", auth.ErrUsernameTaken, user.Username)
		}

		return fmt.Errorf("insert user: %w", err)
	}

	return nil
}

func (r *Repository) UpdateUser(ctx context.Context, user *auth.User) error {
	result, err := r.builder().
		Update(tableUsers).
		Set(userColumnUsername, user.Username).
		Set(userColumnName, user.Name).
		Set(userColumnPasswordHash, user.PasswordHash).
		Set(userColumnUpdatedAt, formatTime(user.UpdatedAt)).
		Where(squirrel.Eq{userColumnID: user.ID}).
		ExecContext(ctx)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("%w: %s", auth.ErrUsernameTaken, user.Username)
		}

		return fmt.Errorf("update user: %w", err)
	}

	return requireOneRow(result, fmt.Errorf("%w: %s", auth.ErrUserNotFound, user.ID))
}

func (r *Repository) DeleteUser(ctx context.Context, id string) error {
	result, err := r.builder().
		Delete(tableUsers).
		Where(squirrel.Eq{userColumnID: id}).
		ExecContext(ctx)
	if err != nil {
		return fmt.Errorf("delete user: %w", err)
	}

	return requireOneRow(result, fmt.Errorf("%w: %s", auth.ErrUserNotFound, id))
}

func (r *Repository) GetUser(ctx context.Context, id string) (*auth.User, error) {
	row := r.builder().
		Select(userColumns()...).
		From(tableUsers).
		Where(squirrel.Eq{userColumnID: id}).
		QueryRowContext(ctx)

	return scanUser(row, fmt.Errorf("%w: %s", auth.ErrUserNotFound, id))
}

func (r *Repository) GetUserByUsername(ctx context.Context, username string) (*auth.User, error) {
	row := r.builder().
		Select(userColumns()...).
		From(tableUsers).
		Where(squirrel.Eq{userColumnUsername: username}).
		QueryRowContext(ctx)

	return scanUser(row, fmt.Errorf("%w: %s", auth.ErrUserNotFound, username))
}

func (r *Repository) ListUsers(ctx context.Context, limit int) ([]auth.User, error) {
	rows, err := r.builder().
		Select(userColumns()...).
		From(tableUsers).
		OrderBy(userColumnCreatedAt, userColumnID).
		Limit(uint64(limit)).
		QueryContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("select users: %w", err)
	}
	defer func() { _ = rows.Close() }()

	users := make([]auth.User, 0)

	for rows.Next() {
		user, err := scanUser(rows, auth.ErrUserNotFound)
		if err != nil {
			return nil, err
		}

		users = append(users, *user)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate users: %w", err)
	}

	return users, nil
}

func (r *Repository) CountUsers(ctx context.Context) (int, error) {
	var count int

	err := r.builder().
		Select("count(*)").
		From(tableUsers).
		QueryRowContext(ctx).
		Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count users: %w", err)
	}

	return count, nil
}

func scanUser(row rowScanner, notFound error) (*auth.User, error) {
	var (
		user                 auth.User
		createdAt, updatedAt string
	)

	err := row.Scan(
		&user.ID,
		&user.Username,
		&user.Name,
		&user.PasswordHash,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, notFound
		}

		return nil, fmt.Errorf("scan user: %w", err)
	}

	if user.CreatedAt, err = parseTime(createdAt); err != nil {
		return nil, fmt.Errorf("parse user created_at: %w", err)
	}

	if user.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return nil, fmt.Errorf("parse user updated_at: %w", err)
	}

	return &user, nil
}
