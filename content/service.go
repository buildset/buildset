package content

import (
	"context"
	"fmt"
	"html/template"
	"strings"
	"time"
	"uuid"

	"github.com/buildset/buildset/pkg/ref"
)

const (
	defaultPostLimit = 50
	maxPostLimit     = 200
)

type Service struct {
	repository Repository
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository}
}

type CreatePostRequest struct {
	AuthorRef   string
	Title       string
	Body        string
	ContentType string
}

type UpdatePostRequest struct {
	Title       string
	Body        string
	ContentType string
}

// CreatePost always produces a draft. Publishing is a separate, deliberate step.
func (s *Service) CreatePost(ctx context.Context, req CreatePostRequest) (*Post, error) {
	if err := ref.Validate(req.AuthorRef); err != nil {
		return nil, fmt.Errorf("%w: author: %w", ErrInvalidPost, err)
	}

	title, err := validateTitle(req.Title)
	if err != nil {
		return nil, err
	}

	contentType := defaultContentType(req.ContentType)
	if err := validateContentType(contentType); err != nil {
		return nil, err
	}

	now := currentTime()

	post := &Post{
		ID:          uuid.NewV7().String(),
		AuthorRef:   req.AuthorRef,
		Title:       title,
		Body:        req.Body,
		ContentType: contentType,
		Status:      StatusDraft,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := s.repository.InsertPost(ctx, post); err != nil {
		return nil, fmt.Errorf("insert post: %w", err)
	}

	return post, nil
}

func (s *Service) UpdatePost(ctx context.Context, id string, req UpdatePostRequest) (*Post, error) {
	post, err := s.GetPost(ctx, id)
	if err != nil {
		return nil, err
	}

	title, err := validateTitle(req.Title)
	if err != nil {
		return nil, err
	}

	contentType := defaultContentType(req.ContentType)
	if err := validateContentType(contentType); err != nil {
		return nil, err
	}

	post.Title = title
	post.Body = req.Body
	post.ContentType = contentType
	post.UpdatedAt = currentTime()

	if err := s.repository.UpdatePost(ctx, post); err != nil {
		return nil, fmt.Errorf("update post: %w", err)
	}

	return post, nil
}

func (s *Service) SetStatus(ctx context.Context, id string, status Status) (*Post, error) {
	post, err := s.GetPost(ctx, id)
	if err != nil {
		return nil, err
	}

	if _, ok := transitions[status]; !ok {
		return nil, fmt.Errorf("%w: %q", ErrInvalidStatus, status)
	}

	if !canTransition(post.Status, status) {
		return nil, fmt.Errorf("%w: %s to %s", ErrInvalidTransition, post.Status, status)
	}

	// The first publication date is part of the post's identity to a reader, so a later
	// republication must not rewrite it.
	if status == StatusPublished && post.PublishedAt == nil {
		published := currentTime()
		post.PublishedAt = &published
	}

	post.Status = status
	post.UpdatedAt = currentTime()

	if err := s.repository.UpdatePost(ctx, post); err != nil {
		return nil, fmt.Errorf("update post status: %w", err)
	}

	return post, nil
}

func (s *Service) DeletePost(ctx context.Context, id string) error {
	if err := s.repository.DeletePost(ctx, id); err != nil {
		return fmt.Errorf("delete post: %w", err)
	}

	return nil
}

func (s *Service) GetPost(ctx context.Context, id string) (*Post, error) {
	post, err := s.repository.GetPost(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get post: %w", err)
	}

	return post, nil
}

func (s *Service) GetPostByRef(ctx context.Context, postRef string) (*Post, error) {
	parsed, err := ref.Parse(postRef)
	if err != nil {
		return nil, err
	}

	if parsed.Service != ServiceName || parsed.ResourceType != PostResourceType {
		return nil, fmt.Errorf("%w: %s is not a post reference", ErrPostNotFound, postRef)
	}

	return s.GetPost(ctx, parsed.ID)
}

func (s *Service) ListPosts(ctx context.Context, filter PostFilter) ([]Post, error) {
	if filter.Status != "" {
		if _, ok := transitions[filter.Status]; !ok {
			return nil, fmt.Errorf("%w: %q", ErrInvalidStatus, filter.Status)
		}
	}

	if filter.Limit <= 0 {
		filter.Limit = defaultPostLimit
	}

	filter.Limit = min(filter.Limit, maxPostLimit)

	posts, err := s.repository.ListPosts(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("list posts: %w", err)
	}

	return posts, nil
}

// RenderBody is the rendering entry point for a stored post.
func (s *Service) RenderBody(post *Post) (template.HTML, error) {
	return RenderHTML(post.ContentType, post.Body)
}

// currentTime is truncated to the precision the stored timestamps keep, so a value held in memory
// and the same value read back from storage compare equal.
func currentTime() time.Time {
	return time.Now().UTC().Truncate(time.Millisecond)
}

func validateTitle(title string) (string, error) {
	trimmed := strings.TrimSpace(title)

	if trimmed == "" {
		return "", fmt.Errorf("%w: a title is required", ErrInvalidPost)
	}

	if len(trimmed) > MaxTitleLength {
		return "", fmt.Errorf("%w: the title must be at most %d characters", ErrInvalidPost, MaxTitleLength)
	}

	return trimmed, nil
}

func defaultContentType(contentType string) string {
	if contentType == "" {
		return ContentTypePlainText
	}

	return contentType
}
