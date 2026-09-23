package app

import (
	"context"
	"html/template"

	"github.com/buildset/buildset/auth"
	authhttpapi "github.com/buildset/buildset/auth/httpapi"
	authui "github.com/buildset/buildset/auth/ui"
	"github.com/buildset/buildset/authz"
	authzhttpapi "github.com/buildset/buildset/authz/httpapi"
	"github.com/buildset/buildset/content"
	contenthttpapi "github.com/buildset/buildset/content/httpapi"
	"github.com/buildset/buildset/pkg/httpx"
	"github.com/buildset/buildset/pkg/ref"
	"github.com/buildset/buildset/web"
)

// Each adapter below wraps a service layer and calls it in process. A remote adapter speaking HTTP
// satisfies the same interface, so nothing in web changes.

type directAuth struct {
	service *auth.Service
	pages   *authui.Handler
}

func (a directAuth) ResolveSession(ctx context.Context, token string) (*web.User, error) {
	_, user, err := a.service.ResolveSession(ctx, token)
	if err != nil {
		return nil, translateAuthError(err)
	}

	return toWebUser(user), nil
}

func (a directAuth) GetUserByRef(ctx context.Context, userRef string) (*web.User, error) {
	user, err := a.service.GetUserByRef(ctx, userRef)
	if err != nil {
		return nil, translateAuthError(err)
	}

	return toWebUser(user), nil
}

func (a directAuth) ListUsers(ctx context.Context, limit int) ([]web.User, error) {
	users, err := a.service.ListUsers(ctx, limit)
	if err != nil {
		return nil, translateAuthError(err)
	}

	converted := make([]web.User, 0, len(users))
	for _, user := range users {
		converted = append(converted, *toWebUser(&user))
	}

	return converted, nil
}

func (a directAuth) UpdateProfile(ctx context.Context, userRef, username, name string) (*web.User, error) {
	parsed, err := ref.Parse(userRef)
	if err != nil {
		return nil, web.NewError(web.ErrNotFound, "That account could not be found.")
	}

	user, err := a.service.UpdateProfile(ctx, auth.UpdateProfileRequest{UserID: parsed.ID, Username: username, Name: name})
	if err != nil {
		return nil, translateAuthError(err)
	}

	return toWebUser(user), nil
}

func (a directAuth) DeleteUser(ctx context.Context, userRef string) error {
	parsed, err := ref.Parse(userRef)
	if err != nil {
		return web.NewError(web.ErrNotFound, "That account could not be found.")
	}

	return translateAuthError(a.service.DeleteUser(ctx, parsed.ID))
}

func (a directAuth) SetupOpen(ctx context.Context) (bool, error) {
	return a.service.SetupOpen(ctx)
}

func (a directAuth) LoginURL(next string) string { return a.pages.LoginURL(next) }
func (a directAuth) LogoutURL() string           { return a.pages.LogoutURL() }
func (a directAuth) PasswordURL() string         { return a.pages.PasswordURL() }
func (a directAuth) RegisterURL() string         { return a.pages.RegisterURL() }

type directAuthz struct {
	service *authz.Service
}

func (a directAuthz) Can(ctx context.Context, subject, action, resource string) (bool, error) {
	allowed, err := a.service.Can(ctx, subject, action, resource)

	return allowed, translateAuthzError(err)
}

func (a directAuthz) Grant(ctx context.Context, subject string, actions []string, resource string) error {
	return translateAuthzError(a.service.Grant(ctx, subject, actions, resource))
}

func (a directAuthz) AssignRole(ctx context.Context, subject, role string) error {
	return translateAuthzError(a.service.AssignRole(ctx, subject, role))
}

func (a directAuthz) RevokeRole(ctx context.Context, subject, role string) error {
	return translateAuthzError(a.service.RevokeRole(ctx, subject, role))
}

func (a directAuthz) SubjectRoles(ctx context.Context, subject string) ([]string, error) {
	roles, err := a.service.SubjectRoles(ctx, subject)

	return roles, translateAuthzError(err)
}

func (a directAuthz) ListRoles(ctx context.Context) ([]string, error) {
	roles, err := a.service.ListRoles(ctx)
	if err != nil {
		return nil, translateAuthzError(err)
	}

	names := make([]string, 0, len(roles))
	for _, role := range roles {
		names = append(names, role.Name)
	}

	return names, nil
}

func (a directAuthz) PurgeResource(ctx context.Context, resource string) error {
	return translateAuthzError(a.service.PurgeResource(ctx, resource))
}

func (a directAuthz) PurgeSubject(ctx context.Context, subject string) error {
	return translateAuthzError(a.service.PurgeSubject(ctx, subject))
}

type directContent struct {
	service *content.Service
}

func (a directContent) GetPost(ctx context.Context, id string) (*web.Post, error) {
	post, err := a.service.GetPost(ctx, id)
	if err != nil {
		return nil, translateContentError(err)
	}

	return toWebPost(post), nil
}

func (a directContent) ListPosts(ctx context.Context, status, authorRef string, limit int) ([]web.Post, error) {
	posts, err := a.service.ListPosts(ctx, content.PostFilter{
		Status:    content.Status(status),
		AuthorRef: authorRef,
		Limit:     limit,
	})
	if err != nil {
		return nil, translateContentError(err)
	}

	converted := make([]web.Post, 0, len(posts))
	for _, post := range posts {
		converted = append(converted, *toWebPost(&post))
	}

	return converted, nil
}

func (a directContent) CreatePost(ctx context.Context, authorRef, title, body, contentType string) (*web.Post, error) {
	post, err := a.service.CreatePost(ctx, content.CreatePostRequest{
		AuthorRef:   authorRef,
		Title:       title,
		Body:        body,
		ContentType: contentType,
	})
	if err != nil {
		return nil, translateContentError(err)
	}

	return toWebPost(post), nil
}

func (a directContent) UpdatePost(ctx context.Context, id, title, body, contentType string) (*web.Post, error) {
	post, err := a.service.UpdatePost(ctx, id, content.UpdatePostRequest{
		Title:       title,
		Body:        body,
		ContentType: contentType,
	})
	if err != nil {
		return nil, translateContentError(err)
	}

	return toWebPost(post), nil
}

func (a directContent) SetStatus(ctx context.Context, id, status string) (*web.Post, error) {
	post, err := a.service.SetStatus(ctx, id, content.Status(status))
	if err != nil {
		return nil, translateContentError(err)
	}

	return toWebPost(post), nil
}

func (a directContent) DeletePost(ctx context.Context, id string) error {
	return translateContentError(a.service.DeletePost(ctx, id))
}

func (a directContent) RenderBody(_ context.Context, post *web.Post) (template.HTML, error) {
	rendered, err := content.RenderHTML(post.ContentType, post.Body)
	if err != nil {
		return "", translateContentError(err)
	}

	return rendered, nil
}

func toWebUser(user *auth.User) *web.User {
	return &web.User{Ref: user.Ref(), ID: user.ID, Username: user.Username, Name: user.Name}
}

func toWebPost(post *content.Post) *web.Post {
	return &web.Post{
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

// The translators below classify with the same functions the HTTP APIs use, so this binary and the
// split one turn a given failure into the same sentinel and sentence. Anything unclassified passes
// through, and the site treats it as a failure of the system.

func translateAuthError(err error) error {
	return translate(err, authhttpapi.Classify)
}

func translateContentError(err error) error {
	return translate(err, contenthttpapi.Classify)
}

func translateAuthzError(err error) error {
	return translate(err, authzhttpapi.Classify)
}

func translate(err error, classify func(error) (httpx.Code, string, bool)) error {
	if err == nil {
		return nil
	}

	code, message, ok := classify(err)
	if !ok {
		return err
	}

	if translated := web.ErrorFromCode(code, message); translated != nil {
		return translated
	}

	return err
}
