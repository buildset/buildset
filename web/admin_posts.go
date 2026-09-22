package web

import (
	"errors"
	"log/slog"
	"net/http"
)

type adminPostsContent struct {
	Posts []Post
	// ShowingEveryone tells an author that the list is theirs rather than the whole site's.
	ShowingEveryone bool
}

type postFormContent struct {
	Post     Post
	Editing  bool
	Statuses []string
}

func (s *Server) adminPosts(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}

	// Someone who may change any post sees every post. Everyone else sees their own.
	seesEveryone, err := s.can(r.Context(), user, ActionPostUpdate, anyPostResource)
	if err != nil {
		s.renderInternalError(w, r, err, "check post permission")

		return
	}

	authorRef := user.Ref
	if seesEveryone {
		authorRef = ""
	}

	posts, err := s.deps.Content.ListPosts(r.Context(), "", authorRef, 0)
	if err != nil {
		s.renderInternalError(w, r, err, "list posts")

		return
	}

	data := s.newLayoutData(r, "Posts")
	data.Content = adminPostsContent{Posts: posts, ShowingEveryone: seesEveryone}

	s.render(w, r, http.StatusOK, "admin_posts.gohtml", data)
}

func (s *Server) newPostForm(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requirePermission(w, r, ActionPostCreate, anyPostResource); !ok {
		return
	}

	data := s.newLayoutData(r, "New post")
	data.Content = postFormContent{Post: Post{ContentType: contentTypePlainText}}

	s.render(w, r, http.StatusOK, "admin_post_form.gohtml", data)
}

// createPost writes the post and then grants its author the rights over it. Ownership is a set of
// grants rather than a comparison in a handler, which is what keeps authorization in one service.
func (s *Server) createPost(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requirePermission(w, r, ActionPostCreate, anyPostResource)
	if !ok {
		return
	}

	if !s.parseForm(w, r) {
		return
	}

	submitted := Post{
		Title:       r.PostFormValue("title"),
		Body:        r.PostFormValue("body"),
		ContentType: contentTypePlainText,
	}

	post, err := s.deps.Content.CreatePost(r.Context(), user.Ref, submitted.Title, submitted.Body, submitted.ContentType)
	if err != nil {
		s.renderPostFormError(w, r, err, "New post", submitted, false)

		return
	}

	// This one cannot be reordered: the grant needs the post's reference, which only exists once
	// the post does. Granting is idempotent, so a single retry is safe and covers a dependency
	// that was briefly unreachable.
	//
	// If it still fails, the post exists and its author cannot edit it. Saying "something went
	// wrong" would invite them to submit the form again and create a second post, so the message
	// says what actually happened and what to do about it.
	if err := s.grantPostOwnership(r, user.Ref, post.Ref); err != nil {
		s.logger.ErrorContext(r.Context(), "grant post ownership",
			slog.String("user_ref", user.Ref),
			slog.String("post_ref", post.Ref),
			slog.Any("error", err),
		)

		s.renderError(w, r, http.StatusInternalServerError,
			"The post was saved, but its permissions were not set. Open it from the post list and try again.")

		return
	}

	http.Redirect(w, r, "/admin/posts/"+post.ID+"/edit", http.StatusSeeOther)
}

func (s *Server) editPostForm(w http.ResponseWriter, r *http.Request) {
	post, ok := s.authorizePost(w, r, ActionPostUpdate)
	if !ok {
		return
	}

	data := s.newLayoutData(r, "Edit post")
	if r.URL.Query().Has("saved") {
		data.Notice = "Your changes have been saved."
	}

	data.Content = postFormContent{Post: *post, Editing: true, Statuses: nextStatuses(post.Status)}

	s.render(w, r, http.StatusOK, "admin_post_form.gohtml", data)
}

func (s *Server) updatePost(w http.ResponseWriter, r *http.Request) {
	post, ok := s.authorizePost(w, r, ActionPostUpdate)
	if !ok {
		return
	}

	if !s.parseForm(w, r) {
		return
	}

	submitted := *post
	submitted.Title = r.PostFormValue("title")
	submitted.Body = r.PostFormValue("body")

	if _, err := s.deps.Content.UpdatePost(r.Context(), post.ID, submitted.Title, submitted.Body, contentTypePlainText); err != nil {
		s.renderPostFormError(w, r, err, "Edit post", submitted, true)

		return
	}

	http.Redirect(w, r, "/admin/posts/"+post.ID+"/edit?saved", http.StatusSeeOther)
}

func (s *Server) setPostStatus(w http.ResponseWriter, r *http.Request) {
	post, ok := s.authorizePost(w, r, ActionPostPublish)
	if !ok {
		return
	}

	if !s.parseForm(w, r) {
		return
	}

	if _, err := s.deps.Content.SetStatus(r.Context(), post.ID, r.PostFormValue("status")); err != nil {
		if !errors.Is(err, ErrInvalidInput) {
			s.renderInternalError(w, r, err, "set post status")

			return
		}

		data := s.newLayoutData(r, "Edit post")
		data.ErrorMessage = err.Error()
		data.Content = postFormContent{Post: *post, Editing: true, Statuses: nextStatuses(post.Status)}

		s.render(w, r, http.StatusBadRequest, "admin_post_form.gohtml", data)

		return
	}

	http.Redirect(w, r, "/admin/posts/"+post.ID+"/edit", http.StatusSeeOther)
}

func (s *Server) deletePost(w http.ResponseWriter, r *http.Request) {
	post, ok := s.authorizePost(w, r, ActionPostDelete)
	if !ok {
		return
	}

	// The grants go first, and the post second. These are two services and cannot share a
	// transaction, so one of them will be left standing if the other fails; this is the order
	// where that state is recoverable. A post that still exists with its grants gone can be
	// deleted again, and an administrator's access comes from their role rather than from these
	// grants, so they can still reach it. The other order leaves grants behind with nothing left
	// to name them.
	//
	// Grants pointing at a resource that no longer exists would eventually match a reused
	// identifier, so they go with it.
	if err := s.deps.Authz.PurgeResource(r.Context(), post.Ref); err != nil {
		s.renderInternalError(w, r, err, "purge post grants")

		return
	}

	if err := s.deps.Content.DeletePost(r.Context(), post.ID); err != nil {
		s.renderInternalError(w, r, err, "delete post")

		return
	}

	http.Redirect(w, r, "/admin/posts", http.StatusSeeOther)
}

// authorizePost loads the post named in the path and checks one permission over it. A post that
// does not exist and one the visitor may not touch both answer the same way.
func (s *Server) authorizePost(w http.ResponseWriter, r *http.Request, action string) (*Post, bool) {
	id := r.PathValue("id")

	resource, err := postResource(id)
	if err != nil {
		s.renderError(w, r, http.StatusNotFound, "There is no such post.")

		return nil, false
	}

	if _, ok := s.requirePermission(w, r, action, resource); !ok {
		return nil, false
	}

	post, err := s.deps.Content.GetPost(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			s.renderError(w, r, http.StatusNotFound, "There is no such post.")

			return nil, false
		}

		s.renderInternalError(w, r, err, "get post")

		return nil, false
	}

	return post, true
}

func (s *Server) renderPostFormError(w http.ResponseWriter, r *http.Request, err error, title string, post Post, editing bool) {
	if !errors.Is(err, ErrInvalidInput) {
		s.renderInternalError(w, r, err, "save post")

		return
	}

	data := s.newLayoutData(r, title)
	data.ErrorMessage = err.Error()
	data.Content = postFormContent{Post: post, Editing: editing, Statuses: nextStatuses(post.Status)}

	s.render(w, r, http.StatusBadRequest, "admin_post_form.gohtml", data)
}

// nextStatuses mirrors the content service's lifecycle so the form offers only moves it accepts.
// The service enforces the rule; this only keeps the page from offering a button that fails.
func nextStatuses(current string) []string {
	switch current {
	case StatusDraft:
		return []string{StatusPublished, StatusArchived}
	case StatusPublished:
		return []string{StatusArchived}
	case StatusArchived:
		return []string{StatusDraft}
	default:
		return nil
	}
}

// grantPostOwnership gives the author the permissions over their own post, retrying once because
// the operation is idempotent and a post nobody can edit is worse than a repeated call.
func (s *Server) grantPostOwnership(r *http.Request, userRef, postRef string) error {
	var err error

	for range 2 {
		if err = s.deps.Authz.Grant(r.Context(), userRef, ownershipActions, postRef); err == nil {
			return nil
		}
	}

	return err
}
