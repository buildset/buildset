package web

import (
	"errors"
	"html/template"
	"net/http"
)

type indexContent struct {
	Posts []Post
}

type postContent struct {
	Post Post
	Body template.HTML
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	// An instance with no users has nothing to show and nobody to show it to, so the first visitor
	// is sent to set it up.
	open, err := s.deps.Auth.SetupOpen(r.Context())
	if err != nil {
		s.renderInternalError(w, r, err, "check setup state")

		return
	}

	if open {
		http.Redirect(w, r, "/setup", http.StatusSeeOther)

		return
	}

	posts, err := s.deps.Content.ListPosts(r.Context(), StatusPublished, "", 0)
	if err != nil {
		s.renderInternalError(w, r, err, "list published posts")

		return
	}

	data := s.newLayoutData(r, s.config.SiteTitle)
	data.Content = indexContent{Posts: posts}

	s.render(w, r, http.StatusOK, "index.gohtml", data)
}

func (s *Server) showPost(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	resource, err := postResource(id)
	if err != nil {
		s.renderError(w, r, http.StatusNotFound, "There is no such post.")

		return
	}

	post, err := s.deps.Content.GetPost(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			s.renderError(w, r, http.StatusNotFound, "There is no such post.")

			return
		}

		s.renderInternalError(w, r, err, "get post")

		return
	}

	if post.Status != StatusPublished {
		user, _ := userFromContext(r.Context())

		allowed, err := s.can(r.Context(), user, ActionPostRead, resource)
		if err != nil {
			s.renderInternalError(w, r, err, "check post access")

			return
		}

		if !allowed {
			// Not 403. A refusal would confirm that a draft with this identifier exists.
			s.logDenied(r, ActionPostRead, resource)
			s.renderError(w, r, http.StatusNotFound, "There is no such post.")

			return
		}
	}

	body, err := s.deps.Content.RenderBody(r.Context(), post)
	if err != nil {
		s.renderInternalError(w, r, err, "render post body")

		return
	}

	data := s.newLayoutData(r, post.Title)
	data.Content = postContent{Post: *post, Body: body}

	s.render(w, r, http.StatusOK, "post.gohtml", data)
}
