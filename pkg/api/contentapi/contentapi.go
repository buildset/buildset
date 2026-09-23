// Package contentapi is the wire shape of the content service's API: types and paths only, so the
// service and its client share one definition without either importing the other.
package contentapi

import "time"

// Rendering a body is not here: it is a pure function, so a consumer renders locally through
// content/render.
const (
	PathGetPost    = "/v1/get-post"
	PathListPosts  = "/v1/list-posts"
	PathCreatePost = "/v1/create-post"
	PathUpdatePost = "/v1/update-post"
	PathSetStatus  = "/v1/set-status"
	PathDeletePost = "/v1/delete-post"
)

type Post struct {
	// Ref is built by the owning service: a consumer uses it as an authorization key and must not
	// decide its shape.
	Ref         string     `json:"ref"`
	ID          string     `json:"id"`
	AuthorRef   string     `json:"author_ref"`
	Title       string     `json:"title"`
	Body        string     `json:"body"`
	ContentType string     `json:"content_type"`
	Status      string     `json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	PublishedAt *time.Time `json:"published_at,omitempty"`
}

type PostResponse struct {
	Post Post `json:"post"`
}

type GetPostRequest struct {
	ID string `json:"id"`
}

type ListPostsRequest struct {
	Status    string `json:"status"`
	AuthorRef string `json:"author_ref"`
	Limit     int    `json:"limit"`
}

type ListPostsResponse struct {
	Posts []Post `json:"posts"`
}

type CreatePostRequest struct {
	AuthorRef   string `json:"author_ref"`
	Title       string `json:"title"`
	Body        string `json:"body"`
	ContentType string `json:"content_type"`
}

type UpdatePostRequest struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Body        string `json:"body"`
	ContentType string `json:"content_type"`
}

type SetStatusRequest struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

type DeletePostRequest struct {
	ID string `json:"id"`
}

// Empty is the request or response of an operation that carries nothing.
type Empty struct{}
