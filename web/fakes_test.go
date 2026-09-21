package web_test

import (
	"context"
	"html/template"
	"slices"
	"strings"

	"github.com/buildset/buildset/web"
)

// The fakes below stand in for the other services. They are hand written rather than generated:
// there are three of them, they are short, and a generated mock would hide what each test is
// actually arranging.

type fakeAuth struct {
	setupOpen bool
	// sessions maps a token to the user it belongs to.
	sessions map[string]*web.User
	users    map[string]*web.User
	deleted  []string
	failWith error
}

func (f *fakeAuth) ResolveSession(_ context.Context, token string) (*web.User, error) {
	if f.failWith != nil {
		return nil, f.failWith
	}

	user, ok := f.sessions[token]
	if !ok {
		return nil, web.NewError(web.ErrNotFound, "no session")
	}

	return user, nil
}

func (f *fakeAuth) GetUserByRef(_ context.Context, userRef string) (*web.User, error) {
	user, ok := f.users[userRef]
	if !ok {
		return nil, web.NewError(web.ErrNotFound, "no user")
	}

	return user, nil
}

func (f *fakeAuth) ListUsers(context.Context, int) ([]web.User, error) {
	users := make([]web.User, 0, len(f.users))
	for _, user := range f.users {
		users = append(users, *user)
	}

	return users, nil
}

func (f *fakeAuth) UpdateProfile(_ context.Context, userRef, username, name string) (*web.User, error) {
	user, ok := f.users[userRef]
	if !ok {
		return nil, web.NewError(web.ErrNotFound, "no user")
	}

	user.Username = username
	user.Name = name

	return user, nil
}

func (f *fakeAuth) DeleteUser(_ context.Context, userRef string) error {
	if _, ok := f.users[userRef]; !ok {
		return web.NewError(web.ErrNotFound, "no user")
	}

	delete(f.users, userRef)
	f.deleted = append(f.deleted, userRef)

	return nil
}

func (f *fakeAuth) SetupOpen(context.Context) (bool, error) { return f.setupOpen, nil }
func (f *fakeAuth) LoginURL(next string) string             { return "/login?next=" + next }
func (f *fakeAuth) LogoutURL() string                       { return "/logout" }
func (f *fakeAuth) PasswordURL() string                     { return "/password" }
func (f *fakeAuth) RegisterURL() string                     { return "/register" }

type grantCall struct {
	Subject  string
	Actions  []string
	Resource string
}

type fakeAuthz struct {
	// permissions holds "subject|action|resource" keys that are allowed.
	permissions map[string]bool
	roles       map[string][]string
	grants      []grantCall
	purged      []string
}

func (f *fakeAuthz) Can(_ context.Context, subject, action, resource string) (bool, error) {
	return f.permissions[subject+"|"+action+"|"+resource], nil
}

func (f *fakeAuthz) Grant(_ context.Context, subject string, actions []string, resource string) error {
	f.grants = append(f.grants, grantCall{Subject: subject, Actions: actions, Resource: resource})

	return nil
}

func (f *fakeAuthz) AssignRole(_ context.Context, subject, role string) error {
	if !slices.Contains(f.roles[subject], role) {
		f.roles[subject] = append(f.roles[subject], role)
	}

	return nil
}

func (f *fakeAuthz) RevokeRole(_ context.Context, subject, role string) error {
	f.roles[subject] = slices.DeleteFunc(f.roles[subject], func(held string) bool { return held == role })

	return nil
}

func (f *fakeAuthz) SubjectRoles(_ context.Context, subject string) ([]string, error) {
	return f.roles[subject], nil
}

func (f *fakeAuthz) ListRoles(context.Context) ([]string, error) {
	return []string{"admin", "author", "reader"}, nil
}

func (f *fakeAuthz) PurgeResource(_ context.Context, resource string) error {
	f.purged = append(f.purged, resource)

	return nil
}

func (f *fakeAuthz) PurgeSubject(_ context.Context, subject string) error {
	f.purged = append(f.purged, subject)

	return nil
}

func (f *fakeAuthz) allow(subject, action, resource string) {
	f.permissions[subject+"|"+action+"|"+resource] = true
}

type fakeContent struct {
	posts   map[string]*web.Post
	created []web.Post
	deleted []string
}

func (f *fakeContent) GetPost(_ context.Context, id string) (*web.Post, error) {
	post, ok := f.posts[id]
	if !ok {
		return nil, web.NewError(web.ErrNotFound, "no post")
	}

	return post, nil
}

func (f *fakeContent) ListPosts(_ context.Context, status, authorRef string, _ int) ([]web.Post, error) {
	var posts []web.Post

	for _, post := range f.posts {
		if status != "" && post.Status != status {
			continue
		}

		if authorRef != "" && post.AuthorRef != authorRef {
			continue
		}

		posts = append(posts, *post)
	}

	return posts, nil
}

func (f *fakeContent) CreatePost(_ context.Context, authorRef, title, body, contentType string) (*web.Post, error) {
	if strings.TrimSpace(title) == "" {
		return nil, web.NewError(web.ErrInvalidInput, "a title is required")
	}

	post := &web.Post{
		Ref:         "urn:content:post:new-post",
		ID:          "new-post",
		AuthorRef:   authorRef,
		Title:       title,
		Body:        body,
		ContentType: contentType,
		Status:      web.StatusDraft,
	}

	f.posts[post.ID] = post
	f.created = append(f.created, *post)

	return post, nil
}

func (f *fakeContent) UpdatePost(_ context.Context, id, title, body, contentType string) (*web.Post, error) {
	post, ok := f.posts[id]
	if !ok {
		return nil, web.NewError(web.ErrNotFound, "no post")
	}

	post.Title = title
	post.Body = body
	post.ContentType = contentType

	return post, nil
}

func (f *fakeContent) SetStatus(_ context.Context, id, status string) (*web.Post, error) {
	post, ok := f.posts[id]
	if !ok {
		return nil, web.NewError(web.ErrNotFound, "no post")
	}

	post.Status = status

	return post, nil
}

func (f *fakeContent) DeletePost(_ context.Context, id string) error {
	if _, ok := f.posts[id]; !ok {
		return web.NewError(web.ErrNotFound, "no post")
	}

	delete(f.posts, id)
	f.deleted = append(f.deleted, id)

	return nil
}

func (f *fakeContent) RenderBody(_ context.Context, post *web.Post) (template.HTML, error) {
	return template.HTML("<p>" + template.HTMLEscapeString(post.Body) + "</p>"), nil
}
