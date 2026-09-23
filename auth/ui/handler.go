// Package ui serves the browser pages that must handle a password: sign in, register, first-run
// setup, and password change. No other service renders these, so no other service ever receives a
// password from a browser.
package ui

import (
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net"
	"net/http"
	"net/url"

	"github.com/buildset/buildset/auth"
	"github.com/buildset/buildset/pkg/safeurl"
)

// maxFormBytes caps a form submission; these forms are a handful of short fields.
const maxFormBytes = 16 << 10

var (
	errInvalidConfig     = errors.New("invalid configuration")
	errMissingDependency = errors.New("missing dependency")
)

type Config struct {
	SessionCookieName string
	SecureCookies     bool
}

type Handler struct {
	service   *auth.Service
	policy    RegistrationPolicy
	config    Config
	templates map[string]*template.Template
	logger    *slog.Logger
}

func NewHandler(
	service *auth.Service,
	policy RegistrationPolicy,
	config Config,
	logger *slog.Logger,
) (*Handler, error) {
	if config.SessionCookieName == "" {
		return nil, fmt.Errorf("%w: session cookie name must not be empty", errInvalidConfig)
	}

	if policy == nil {
		return nil, fmt.Errorf("%w: a registration policy must be provided", errMissingDependency)
	}

	templates, err := parseTemplates()
	if err != nil {
		return nil, err
	}

	return &Handler{
		service:   service,
		policy:    policy,
		config:    config,
		templates: templates,
		logger:    logger,
	}, nil
}

// Register mounts auth's pages at top-level paths, so moving auth to its own host later is a
// configuration change and not a change to any link.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /setup", h.setupForm)
	mux.HandleFunc("POST /setup", h.setupSubmit)
	mux.HandleFunc("GET /login", h.loginForm)
	mux.HandleFunc("POST /login", h.loginSubmit)
	mux.HandleFunc("POST /logout", h.logoutSubmit)
	mux.HandleFunc("GET /register", h.registerForm)
	mux.HandleFunc("POST /register", h.registerSubmit)
	mux.HandleFunc("GET /password", h.passwordForm)
	mux.HandleFunc("POST /password", h.passwordSubmit)
}

// LoginURL keeps the rest of the system out of auth's routing. An OAuth2 authorization endpoint
// would be returned from here instead.
func (h *Handler) LoginURL(next string) string {
	target := safeurl.Next(next)
	if target == "/" {
		return "/login"
	}

	return "/login?next=" + url.QueryEscape(target)
}

func (h *Handler) LogoutURL() string {
	return "/logout"
}

func (h *Handler) PasswordURL() string {
	return "/password"
}

func (h *Handler) RegisterURL() string {
	return "/register"
}

// parseForm bounds the request body before reading it, so a large upload cannot be turned into
// memory pressure by an unauthenticated client.
func (h *Handler) parseForm(w http.ResponseWriter, r *http.Request) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxFormBytes)

	if err := r.ParseForm(); err != nil {
		h.renderError(w, r, http.StatusBadRequest, "That form could not be read.")

		return false
	}

	return true
}

func (h *Handler) sessionMeta(r *http.Request) auth.SessionMeta {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}

	// TODO: behind a reverse proxy this records the proxy. Reading X-Forwarded-For needs a list of
	// trusted proxies first, otherwise any client can write whatever address it likes.
	return auth.SessionMeta{UserAgent: r.UserAgent(), IP: host}
}

// startSession replaces whatever session the browser presented, so there is no identifier for an
// attacker to fix in advance.
func (h *Handler) startSession(w http.ResponseWriter, r *http.Request, user *auth.User) error {
	if presented := h.sessionToken(r); presented != "" {
		if err := h.service.RevokeSession(r.Context(), presented); err != nil {
			return fmt.Errorf("revoke presented session: %w", err)
		}
	}

	token, session, err := h.service.CreateSession(r.Context(), user.ID, h.sessionMeta(r))
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}

	h.setSessionCookie(w, token, session.ExpiresAt)

	return nil
}

// An absent or expired session is not an error here; the caller decides what to do about it.
func (h *Handler) currentSession(r *http.Request) (*auth.Session, *auth.User, error) {
	token := h.sessionToken(r)
	if token == "" {
		return nil, nil, auth.ErrSessionNotFound
	}

	return h.service.ResolveSession(r.Context(), token)
}
