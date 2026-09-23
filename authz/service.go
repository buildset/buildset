package authz

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/buildset/buildset/pkg/ref"
)

type Service struct {
	repository Repository
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository}
}

// Can reports whether the subject holds a permission covering this action on this resource.
//
// TODO: every pattern a subject holds is loaded on each check. That is fine while a subject has
// tens of grants; push matching into the backend before one can hold thousands.
func (s *Service) Can(ctx context.Context, subject, action, resource string) (bool, error) {
	if err := validateSubject(subject); err != nil {
		return false, err
	}

	if err := validateAction(action); err != nil {
		return false, err
	}

	// A query may name a class of resource rather than one of them, which is how a caller asks
	// "may this subject create posts at all" before any post exists.
	if err := validateResourceQuery(resource); err != nil {
		return false, err
	}

	patterns, err := s.repository.SubjectPatterns(ctx, subject)
	if err != nil {
		return false, fmt.Errorf("load subject patterns: %w", err)
	}

	return allows(patterns, action, resource), nil
}

func (s *Service) ListRoles(ctx context.Context) ([]Role, error) {
	roles, err := s.repository.ListRoles(ctx)
	if err != nil {
		return nil, fmt.Errorf("list roles: %w", err)
	}

	return roles, nil
}

func (s *Service) SubjectRoles(ctx context.Context, subject string) ([]string, error) {
	if err := validateSubject(subject); err != nil {
		return nil, err
	}

	roles, err := s.repository.SubjectRoles(ctx, subject)
	if err != nil {
		return nil, fmt.Errorf("list subject roles: %w", err)
	}

	return roles, nil
}

func (s *Service) AssignRole(ctx context.Context, subject, role string) error {
	if err := validateSubject(subject); err != nil {
		return err
	}

	exists, err := s.repository.RoleExists(ctx, role)
	if err != nil {
		return fmt.Errorf("check role: %w", err)
	}

	if !exists {
		return fmt.Errorf("%w: %s", ErrUnknownRole, role)
	}

	if err := s.repository.InsertSubjectRole(ctx, subject, role, time.Now().UTC()); err != nil {
		return fmt.Errorf("assign role: %w", err)
	}

	return nil
}

func (s *Service) RevokeRole(ctx context.Context, subject, role string) error {
	if err := validateSubject(subject); err != nil {
		return err
	}

	if err := s.repository.DeleteSubjectRole(ctx, subject, role); err != nil {
		return fmt.Errorf("revoke role: %w", err)
	}

	return nil
}

// Grant is how ownership is expressed: the service that creates a resource grants its creator the
// actions over it.
func (s *Service) Grant(ctx context.Context, subject string, actions []string, resource string) error {
	if err := validateSubject(subject); err != nil {
		return err
	}

	if err := validateResource(resource); err != nil {
		return err
	}

	if len(actions) == 0 {
		return fmt.Errorf("%w: no actions given", ErrInvalidAction)
	}

	for _, action := range actions {
		if err := validateAction(action); err != nil {
			return err
		}
	}

	if err := s.repository.InsertGrants(ctx, subject, actions, resource, time.Now().UTC()); err != nil {
		return fmt.Errorf("insert grants: %w", err)
	}

	return nil
}

// PurgeSubject removes everything held by a subject, for when whatever it references is deleted.
func (s *Service) PurgeSubject(ctx context.Context, subject string) error {
	if err := validateSubject(subject); err != nil {
		return err
	}

	if err := s.repository.DeleteBySubject(ctx, subject); err != nil {
		return fmt.Errorf("purge subject: %w", err)
	}

	return nil
}

// PurgeResource removes every grant over a resource, for when that resource is deleted.
func (s *Service) PurgeResource(ctx context.Context, resource string) error {
	if err := validateResource(resource); err != nil {
		return err
	}

	if err := s.repository.DeleteByResource(ctx, resource); err != nil {
		return fmt.Errorf("purge resource: %w", err)
	}

	return nil
}

// The shape is checked without resolving it: malformed keys are how an authorization table quietly
// stops matching.
func validateSubject(subject string) error {
	if err := ref.Validate(subject); err != nil {
		return fmt.Errorf("subject: %w", err)
	}

	return nil
}

func validateResource(resource string) error {
	if err := ref.Validate(resource); err != nil {
		return fmt.Errorf("resource: %w", err)
	}

	return nil
}

// validateResourceQuery also accepts a wildcard covering a whole service, a whole resource type,
// or everything. A grant still requires one concrete resource, because a stored wildcard grant
// would be a policy rule and this service does not let callers write those.
func validateResourceQuery(resource string) error {
	if resource == wildcard {
		return nil
	}

	prefix, found := strings.CutSuffix(resource, wildcard)
	if !found {
		return validateResource(resource)
	}

	// Check the shape of what precedes the wildcard by standing a placeholder in its place.
	if err := ref.Validate(prefix + "x"); err != nil {
		return fmt.Errorf("resource pattern: %w", err)
	}

	return nil
}

// validateAction accepts the dotted lowercase names services use, such as "post.update".
func validateAction(action string) error {
	if action == "" || len(action) > 64 {
		return fmt.Errorf("%w: must be between 1 and 64 characters", ErrInvalidAction)
	}

	for _, c := range action {
		switch {
		case c >= 'a' && c <= 'z':
		case c >= '0' && c <= '9':
		case c == '.' || c == '_' || c == '-':
		default:
			return fmt.Errorf("%w: %q contains %q", ErrInvalidAction, action, c)
		}
	}

	return nil
}
