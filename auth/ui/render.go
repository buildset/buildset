package ui

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
)

//go:embed templates/*.gohtml
var templateFiles embed.FS

// pageData is what every template in this package receives. One struct keeps the layout's fields
// available on every page without each handler rebuilding them.
type pageData struct {
	Title            string
	ErrorMessage     string
	Notice           string
	Message          string
	Next             string
	Username         string
	Name             string
	RegistrationOpen bool
	// ForSomeoneElse marks a registration form being used to add another person's account.
	ForSomeoneElse bool
}

// parseTemplates builds one template set per page, each with its own copy of the layout, because
// every page defines the same "content" block.
func parseTemplates() (map[string]*template.Template, error) {
	names := []string{"setup.gohtml", "login.gohtml", "register.gohtml", "password.gohtml", "error.gohtml"}
	pages := make(map[string]*template.Template, len(names))

	for _, name := range names {
		page, err := template.ParseFS(templateFiles, "templates/layout.gohtml", "templates/"+name)
		if err != nil {
			return nil, fmt.Errorf("parse template %s: %w", name, err)
		}

		pages[name] = page
	}

	return pages, nil
}

// render writes a page. The template runs into a buffer first so a template failure becomes an
// error page rather than a truncated one sent with a success status.
func (h *Handler) render(w http.ResponseWriter, r *http.Request, status int, page string, data pageData) {
	tmpl, ok := h.templates[page]
	if !ok {
		h.logger.ErrorContext(r.Context(), "unknown template", slog.String("page", page))
		http.Error(w, "internal server error", http.StatusInternalServerError)

		return
	}

	var buf bytes.Buffer

	if err := tmpl.ExecuteTemplate(&buf, "layout.gohtml", data); err != nil {
		h.logger.ErrorContext(r.Context(), "execute template", slog.String("page", page), slog.Any("error", err))
		http.Error(w, "internal server error", http.StatusInternalServerError)

		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// These pages carry credentials and session state, so no cache may keep a copy.
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
}

func (h *Handler) renderError(w http.ResponseWriter, r *http.Request, status int, message string) {
	h.render(w, r, status, "error.gohtml", pageData{Title: http.StatusText(status), Message: message})
}

// renderInternalError hides the cause from the browser and puts it in the log instead.
func (h *Handler) renderInternalError(w http.ResponseWriter, r *http.Request, err error, message string) {
	h.logger.ErrorContext(r.Context(), message, slog.Any("error", err))
	h.renderError(w, r, http.StatusInternalServerError, "Something went wrong. Please try again.")
}
