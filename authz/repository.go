package authz

import (
	"context"
	"time"
)

type Repository interface {
	ListRoles(ctx context.Context) ([]Role, error)
	RoleExists(ctx context.Context, role string) (bool, error)

	// SubjectPatterns returns every permission a subject holds, from roles and direct grants alike.
	// The service decides which of them match.
	SubjectPatterns(ctx context.Context, subject string) ([]Pattern, error)
	SubjectRoles(ctx context.Context, subject string) ([]string, error)

	InsertSubjectRole(ctx context.Context, subject, role string, grantedAt time.Time) error
	DeleteSubjectRole(ctx context.Context, subject, role string) error

	InsertGrants(ctx context.Context, subject string, actions []string, resource string, grantedAt time.Time) error

	DeleteBySubject(ctx context.Context, subject string) error
	DeleteByResource(ctx context.Context, resource string) error
}
