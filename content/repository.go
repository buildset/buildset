package content

import "context"

// PostFilter narrows a listing. A zero value lists everything up to the default limit.
type PostFilter struct {
	// Status is empty to match any status.
	Status Status
	// AuthorRef is empty to match any author.
	AuthorRef string
	Limit     int
}

type Repository interface {
	InsertPost(ctx context.Context, post *Post) error
	UpdatePost(ctx context.Context, post *Post) error
	DeletePost(ctx context.Context, id string) error
	GetPost(ctx context.Context, id string) (*Post, error)
	ListPosts(ctx context.Context, filter PostFilter) ([]Post, error)
}
