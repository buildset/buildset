package web

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"slices"
)

//go:embed templates/*.gohtml
var templateFiles embed.FS

// Listing every page keeps a typo in a handler a startup problem rather than a runtime one.
var pageNames = []string{
	"index.gohtml",
	"post.gohtml",
	"account.gohtml",
	"error.gohtml",
	"admin.gohtml",
	"admin_posts.gohtml",
	"admin_post_form.gohtml",
	"admin_users.gohtml",
	"admin_user.gohtml",
}

// layoutData is what every template receives. Handlers fill Content with the page's own data.
type layoutData struct {
	SiteTitle    string
	Title        string
	CurrentUser  *User
	LoginURL     string
	LogoutURL    string
	PasswordURL  string
	CanSeeAdmin  bool
	ErrorMessage string
	Notice       string
	Content      any
}

// templateFunctions are the few helpers the pages need that html/template does not provide.
var templateFunctions = template.FuncMap{
	"has": slices.Contains[[]string, string],
}

func parseTemplates() (map[string]*template.Template, error) {
	pages := make(map[string]*template.Template, len(pageNames))

	for _, name := range pageNames {
		page, err := template.New(name).Funcs(templateFunctions).ParseFS(templateFiles, "templates/layout.gohtml", "templates/"+name)
		if err != nil {
			return nil, fmt.Errorf("parse template %s: %w", name, err)
		}

		pages[name] = page
	}

	return pages, nil
}

// newLayoutData fills in what the layout needs on every page, so no handler has to remember it.
func (s *Server) newLayoutData(r *http.Request, title string) layoutData {
	user, _ := userFromContext(r.Context())

	data := layoutData{
		SiteTitle:   s.config.SiteTitle,
		Title:       title,
		CurrentUser: user,
		LoginURL:    s.deps.Auth.LoginURL(r.URL.RequestURI()),
		LogoutURL:   s.deps.Auth.LogoutURL(),
		PasswordURL: s.deps.Auth.PasswordURL(),
	}

	if user != nil {
		// A failure to answer hides the administration link rather than blocking the page.
		allowed, err := s.can(r.Context(), user, ActionPostCreate, anyPostResource)
		if err != nil {
			s.logger.WarnContext(r.Context(), "check admin visibility", slog.Any("error", err))
		}

		data.CanSeeAdmin = allowed
	}

	return data
}

// The template runs into a buffer first, so a failure becomes an error page rather than a truncated
// one already sent with a success status.
func (s *Server) render(w http.ResponseWriter, r *http.Request, status int, page string, data layoutData) {
	tmpl, ok := s.templates[page]
	if !ok {
		s.logger.ErrorContext(r.Context(), "unknown template", slog.String("page", page))
		http.Error(w, "internal server error", http.StatusInternalServerError)

		return
	}

	var buf bytes.Buffer

	if err := tmpl.ExecuteTemplate(&buf, "layout.gohtml", data); err != nil {
		s.logger.ErrorContext(r.Context(), "execute template", slog.String("page", page), slog.Any("error", err))
		http.Error(w, "internal server error", http.StatusInternalServerError)

		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
}

func (s *Server) renderError(w http.ResponseWriter, r *http.Request, status int, message string) {
	data := s.newLayoutData(r, http.StatusText(status))
	data.Content = message

	s.render(w, r, status, "error.gohtml", data)
}

// renderInternalError puts the cause in the log and keeps it out of the response.
func (s *Server) renderInternalError(w http.ResponseWriter, r *http.Request, err error, message string) {
	s.logger.ErrorContext(r.Context(), message, slog.Any("error", err))
	s.renderError(w, r, http.StatusInternalServerError, "Something went wrong. Please try again.")
}
