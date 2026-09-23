// Package web owns the public site and the administration pages. It holds no domain data of its
// own and never handles a password: it composes the other services into pages and links to the
// identity service for anything involving credentials.
package web

import (
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
)

// maxFormBytes caps a form submission; a post body is the largest thing submitted here.
const maxFormBytes = 1 << 20

type Config struct {
	SessionCookieName string
	SecureCookies     bool
	SiteTitle         string
}

// Dependencies are the other services this site composes, each behind an interface it declares.
type Dependencies struct {
	Auth    Auth
	Authz   Authz
	Content Content
}

type Server struct {
	deps      Dependencies
	config    Config
	templates map[string]*template.Template
	logger    *slog.Logger
}

func New(deps Dependencies, config Config, logger *slog.Logger) (*Server, error) {
	if deps.Auth == nil || deps.Authz == nil || deps.Content == nil {
		return nil, fmt.Errorf("every dependency must be provided")
	}

	if config.SessionCookieName == "" {
		return nil, fmt.Errorf("session cookie name must not be empty")
	}

	if config.SiteTitle == "" {
		config.SiteTitle = "Blog"
	}

	templates, err := parseTemplates()
	if err != nil {
		return nil, err
	}

	return &Server{deps: deps, config: config, templates: templates, logger: logger}, nil
}

// Handler returns the site, wrapped in its session middleware.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /{$}", s.index)
	mux.HandleFunc("GET /posts/{id}", s.showPost)
	mux.HandleFunc("GET /account", s.accountForm)
	mux.HandleFunc("POST /account", s.accountSubmit)

	mux.HandleFunc("GET /admin", s.adminIndex)
	mux.HandleFunc("GET /admin/posts", s.adminPosts)
	mux.HandleFunc("GET /admin/posts/new", s.newPostForm)
	mux.HandleFunc("POST /admin/posts", s.createPost)
	mux.HandleFunc("GET /admin/posts/{id}/edit", s.editPostForm)
	mux.HandleFunc("POST /admin/posts/{id}", s.updatePost)
	mux.HandleFunc("POST /admin/posts/{id}/status", s.setPostStatus)
	mux.HandleFunc("POST /admin/posts/{id}/delete", s.deletePost)
	mux.HandleFunc("GET /admin/users", s.adminUsers)
	mux.HandleFunc("GET /admin/users/{id}", s.adminUser)
	mux.HandleFunc("POST /admin/users/{id}/roles", s.setUserRoles)
	mux.HandleFunc("POST /admin/users/{id}/delete", s.deleteUser)

	// Anything this mux does not recognize is a missing page, rendered in the site's layout.
	mux.HandleFunc("/", s.notFound)

	return s.withSession(mux)
}

// parseForm bounds the request body before reading it.
func (s *Server) parseForm(w http.ResponseWriter, r *http.Request) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxFormBytes)

	if err := r.ParseForm(); err != nil {
		s.renderError(w, r, http.StatusBadRequest, "That form could not be read.")

		return false
	}

	return true
}

func (s *Server) notFound(w http.ResponseWriter, r *http.Request) {
	s.renderError(w, r, http.StatusNotFound, "There is nothing here.")
}
