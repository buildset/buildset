// Package httpapi serves the content service over HTTP.
//
// Rendering a body is deliberately not an operation here: it needs the body and its content type
// and nothing else, so a consumer renders locally through content/render instead of spending a
// round trip on a pure function.
package httpapi

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/buildset/buildset/content"
	"github.com/buildset/buildset/pkg/api/contentapi"
	"github.com/buildset/buildset/pkg/httpx"
)

type Handler struct {
	service *content.Service
	logger  *slog.Logger
}

func NewHandler(service *content.Service, logger *slog.Logger) (*Handler, error) {
	if service == nil {
		return nil, fmt.Errorf("content service must not be nil")
	}

	if logger == nil {
		return nil, fmt.Errorf("logger must not be nil")
	}

	return &Handler{service: service, logger: logger}, nil
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST "+contentapi.PathGetPost, h.getPost)
	mux.HandleFunc("POST "+contentapi.PathListPosts, h.listPosts)
	mux.HandleFunc("POST "+contentapi.PathCreatePost, h.createPost)
	mux.HandleFunc("POST "+contentapi.PathUpdatePost, h.updatePost)
	mux.HandleFunc("POST "+contentapi.PathSetStatus, h.setStatus)
	mux.HandleFunc("POST "+contentapi.PathDeletePost, h.deletePost)
}

func (h *Handler) getPost(w http.ResponseWriter, r *http.Request) {
	var request contentapi.GetPostRequest
	if !httpx.DecodeJSON(w, r, &request) {
		return
	}

	post, err := h.service.GetPost(r.Context(), request.ID)
	h.post(w, r, "get post", post, err)
}

func (h *Handler) listPosts(w http.ResponseWriter, r *http.Request) {
	var request contentapi.ListPostsRequest
	if !httpx.DecodeJSON(w, r, &request) {
		return
	}

	posts, err := h.service.ListPosts(r.Context(), content.PostFilter{
		Status:    content.Status(request.Status),
		AuthorRef: request.AuthorRef,
		Limit:     request.Limit,
	})
	if err != nil {
		h.fail(w, r, "list posts", err)

		return
	}

	wire := make([]contentapi.Post, 0, len(posts))
	for _, post := range posts {
		wire = append(wire, toWire(&post))
	}

	httpx.WriteJSON(w, contentapi.ListPostsResponse{Posts: wire})
}

func (h *Handler) createPost(w http.ResponseWriter, r *http.Request) {
	var request contentapi.CreatePostRequest
	if !httpx.DecodeJSON(w, r, &request) {
		return
	}

	post, err := h.service.CreatePost(r.Context(), content.CreatePostRequest{
		AuthorRef:   request.AuthorRef,
		Title:       request.Title,
		Body:        request.Body,
		ContentType: request.ContentType,
	})
	h.post(w, r, "create post", post, err)
}

func (h *Handler) updatePost(w http.ResponseWriter, r *http.Request) {
	var request contentapi.UpdatePostRequest
	if !httpx.DecodeJSON(w, r, &request) {
		return
	}

	post, err := h.service.UpdatePost(r.Context(), request.ID, content.UpdatePostRequest{
		Title:       request.Title,
		Body:        request.Body,
		ContentType: request.ContentType,
	})
	h.post(w, r, "update post", post, err)
}

func (h *Handler) setStatus(w http.ResponseWriter, r *http.Request) {
	var request contentapi.SetStatusRequest
	if !httpx.DecodeJSON(w, r, &request) {
		return
	}

	post, err := h.service.SetStatus(r.Context(), request.ID, content.Status(request.Status))
	h.post(w, r, "set status", post, err)
}

func (h *Handler) deletePost(w http.ResponseWriter, r *http.Request) {
	var request contentapi.DeletePostRequest
	if !httpx.DecodeJSON(w, r, &request) {
		return
	}

	if err := h.service.DeletePost(r.Context(), request.ID); err != nil {
		h.fail(w, r, "delete post", err)

		return
	}

	httpx.WriteJSON(w, contentapi.Empty{})
}

func (h *Handler) post(w http.ResponseWriter, r *http.Request, operation string, post *content.Post, err error) {
	if err != nil {
		h.fail(w, r, operation, err)

		return
	}

	httpx.WriteJSON(w, contentapi.PostResponse{Post: toWire(post)})
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, operation string, err error) {
	if code, message, ok := Classify(err); ok {
		httpx.WriteError(w, code, message)

		return
	}

	httpx.WriteInternal(r.Context(), w, h.logger, operation, err)
}

// toWire builds the reference here rather than leaving it to the consumer, because a consumer uses
// it as an authorization key and this service owns what one looks like.
func toWire(post *content.Post) contentapi.Post {
	return contentapi.Post{
		Ref:         post.Ref(),
		ID:          post.ID,
		AuthorRef:   post.AuthorRef,
		Title:       post.Title,
		Body:        post.Body,
		ContentType: post.ContentType,
		Status:      string(post.Status),
		CreatedAt:   post.CreatedAt,
		UpdatedAt:   post.UpdatedAt,
		PublishedAt: post.PublishedAt,
	}
}
