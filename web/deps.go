package web

import (
	"context"
	"errors"
	"html/template"
	"time"
)

// The interfaces below are what this site needs from the rest of the system. They use types
// declared here rather than another service's structs, so an implementation that talks HTTP can
// satisfy them without importing it. The composition root supplies the implementations.

// User is what this site needs to know about a person. It holds no credentials.
type User struct {
	Ref string
	// ID is the identifier inside Ref, carried separately so links do not have to take a reference
	// apart to build a URL.
	ID       string
	Username string
	Name     string
}

type Post struct {
	Ref         string
	ID          string
	AuthorRef   string
	Title       string
	Body        string
	ContentType string
	Status      string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	PublishedAt *time.Time
}

const contentTypePlainText = "text/plain"

// Strings because they cross a service boundary. The content service owns the lifecycle; this site
// only names the states it shows.
const (
	StatusDraft     = "draft"
	StatusPublished = "published"
	StatusArchived  = "archived"
)

// Errors a dependency may report. Anything else is treated as a failure of the system rather than
// of the request, so an adapter must map its service's errors onto these.
var (
	ErrNotFound     = errors.New("not found")
	ErrInvalidInput = errors.New("invalid input")
	ErrConflict     = errors.New("conflict")
)

type Auth interface {
	// ResolveSession reports ErrNotFound for a token that is unknown or expired.
	ResolveSession(ctx context.Context, token string) (*User, error)
	GetUserByRef(ctx context.Context, userRef string) (*User, error)
	ListUsers(ctx context.Context, limit int) ([]User, error)
	UpdateProfile(ctx context.Context, userRef, username, name string) (*User, error)
	DeleteUser(ctx context.Context, userRef string) error
	SetupOpen(ctx context.Context) (bool, error)

	// RegisterURL is the identity service's form; this site never renders one.
	RegisterURL() string

	// LoginURL is a method rather than a constant so an authorization-code flow could return a very
	// different URL without anything here changing.
	LoginURL(next string) string
	LogoutURL() string
	PasswordURL() string
}

type Authz interface {
	Can(ctx context.Context, subject, action, resource string) (bool, error)
	Grant(ctx context.Context, subject string, actions []string, resource string) error
	AssignRole(ctx context.Context, subject, role string) error
	RevokeRole(ctx context.Context, subject, role string) error
	SubjectRoles(ctx context.Context, subject string) ([]string, error)
	ListRoles(ctx context.Context) ([]string, error)
	PurgeResource(ctx context.Context, resource string) error
	PurgeSubject(ctx context.Context, subject string) error
}

type Content interface {
	GetPost(ctx context.Context, id string) (*Post, error)
	ListPosts(ctx context.Context, status, authorRef string, limit int) ([]Post, error)
	CreatePost(ctx context.Context, authorRef, title, body, contentType string) (*Post, error)
	UpdatePost(ctx context.Context, id, title, body, contentType string) (*Post, error)
	SetStatus(ctx context.Context, id, status string) (*Post, error)
	DeletePost(ctx context.Context, id string) error
	RenderBody(ctx context.Context, post *Post) (template.HTML, error)
}
