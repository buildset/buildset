package content

import (
	"time"

	"github.com/buildset/buildset/pkg/ref"
)

const MaxTitleLength = 200

type Post struct {
	ID string
	// AuthorRef points at a user in another service. This service never dereferences it.
	AuthorRef   string
	Title       string
	Body        string
	ContentType string
	Status      Status
	CreatedAt   time.Time
	UpdatedAt   time.Time
	// PublishedAt is set the first time a post is published and never moved afterwards.
	PublishedAt *time.Time
}

func (p *Post) Ref() string {
	return ref.MustNew(ServiceName, PostResourceType, p.ID).String()
}
