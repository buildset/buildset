package web_test

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/buildset/buildset/web"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	sessionCookieName = "ms_session"

	adaRef    = "urn:auth:user:0199bf3c-7a1e-7c2b-9f10-000000000001"
	adaID     = "0199bf3c-7a1e-7c2b-9f10-000000000001"
	graceRef  = "urn:auth:user:0199bf3c-7a1e-7c2b-9f10-000000000002"
	draftID   = "0199bf3c-7a1e-7c2b-9f10-0000000000aa"
	draftRef  = "urn:content:post:0199bf3c-7a1e-7c2b-9f10-0000000000aa"
	adaToken  = "ada-token"
	graceName = "grace-token"
)

type harness struct {
	handler http.Handler
	auth    *fakeAuth
	authz   *fakeAuthz
	content *fakeContent
}

func newHarness(t *testing.T) *harness {
	t.Helper()

	ada := &web.User{Ref: adaRef, ID: adaID, Username: "ada", Name: "Ada"}
	grace := &web.User{
		Ref:      graceRef,
		ID:       "0199bf3c-7a1e-7c2b-9f10-000000000002",
		Username: "grace",
		Name:     "Grace",
	}

	auth := &fakeAuth{
		sessions: map[string]*web.User{adaToken: ada, graceName: grace},
		users:    map[string]*web.User{adaRef: ada, graceRef: grace},
	}

	authz := &fakeAuthz{permissions: map[string]bool{}, roles: map[string][]string{}}

	content := &fakeContent{posts: map[string]*web.Post{
		draftID: {
			Ref:       draftRef,
			ID:        draftID,
			AuthorRef: adaRef,
			Title:     "A draft",
			Body:      "Hidden.",
			Status:    web.StatusDraft,
		},
	}}

	server, err := web.New(
		web.Dependencies{Auth: auth, Authz: authz, Content: content},
		web.Config{SessionCookieName: sessionCookieName, SiteTitle: "Blog"},
		slog.New(slog.DiscardHandler),
	)
	require.NoError(t, err)

	return &harness{handler: server.Handler(), auth: auth, authz: authz, content: content}
}

func (h *harness) request(
	t *testing.T,
	method, target, token string,
	form url.Values,
) *httptest.ResponseRecorder {
	t.Helper()

	var request *http.Request

	if form == nil {
		request = httptest.NewRequestWithContext(t.Context(), method, target, nil)
	} else {
		request = httptest.NewRequestWithContext(
			t.Context(),
			method,
			target,
			strings.NewReader(form.Encode()),
		)
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	if token != "" {
		request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	}

	recorder := httptest.NewRecorder()
	h.handler.ServeHTTP(recorder, request)

	return recorder
}

func TestAnonymousVisitorIsSentToSignIn(t *testing.T) {
	h := newHarness(t)

	response := h.request(t, http.MethodGet, "/admin/posts/new", "", nil)

	require.Equal(t, http.StatusSeeOther, response.Code)
	assert.Equal(t, "/login?next=/admin/posts/new", response.Header().Get("Location"))
}

// A draft must not be distinguishable from a post that was never written, so the answer is 404 and
// not 403. A 403 would confirm that this identifier names something.
func TestDraftLooksMissingToEveryoneWithoutAccess(t *testing.T) {
	h := newHarness(t)

	assert.Equal(
		t,
		http.StatusNotFound,
		h.request(t, http.MethodGet, "/posts/"+draftID, "", nil).Code,
	)
	assert.Equal(
		t,
		http.StatusNotFound,
		h.request(t, http.MethodGet, "/posts/"+draftID, graceName, nil).Code,
	)

	h.authz.allow(adaRef, web.ActionPostRead, draftRef)
	response := h.request(t, http.MethodGet, "/posts/"+draftID, adaToken, nil)
	require.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Body.String(), "Hidden.")
}

func TestEditingSomeoneElsesPostIsRefused(t *testing.T) {
	h := newHarness(t)

	assert.Equal(
		t,
		http.StatusForbidden,
		h.request(t, http.MethodGet, "/admin/posts/"+draftID+"/edit", graceName, nil).Code,
	)

	h.authz.allow(adaRef, web.ActionPostUpdate, draftRef)
	assert.Equal(
		t,
		http.StatusOK,
		h.request(t, http.MethodGet, "/admin/posts/"+draftID+"/edit", adaToken, nil).Code,
	)
}

// This is where ownership is actually established. Without these grants the author of a new post
// could not read, edit, publish, or delete it afterwards.
func TestCreatingAPostGrantsItsAuthorOwnership(t *testing.T) {
	h := newHarness(t)
	h.authz.allow(adaRef, web.ActionPostCreate, "urn:content:post:*")

	response := h.request(t, http.MethodPost, "/admin/posts", adaToken, url.Values{
		"title": {"A new post"},
		"body":  {"Hello."},
	})

	require.Equal(t, http.StatusSeeOther, response.Code)
	require.Len(t, h.content.created, 1)
	require.Len(t, h.authz.grants, 1)

	grant := h.authz.grants[0]
	assert.Equal(t, adaRef, grant.Subject)
	assert.Equal(t, h.content.created[0].Ref, grant.Resource)
	assert.ElementsMatch(
		t,
		[]string{
			web.ActionPostRead,
			web.ActionPostUpdate,
			web.ActionPostDelete,
			web.ActionPostPublish,
		},
		grant.Actions,
	)
}

func TestDeletingAPostRemovesItsGrants(t *testing.T) {
	h := newHarness(t)
	h.authz.allow(adaRef, web.ActionPostDelete, draftRef)

	response := h.request(
		t,
		http.MethodPost,
		"/admin/posts/"+draftID+"/delete",
		adaToken,
		url.Values{},
	)

	require.Equal(t, http.StatusSeeOther, response.Code)
	assert.Equal(t, []string{draftID}, h.content.deleted)
	assert.Equal(t, []string{draftRef}, h.authz.purged, "grants must not outlive the resource")
}

// An identity service that cannot be reached must never be read as "this visitor is anonymous".
func TestUnreachableIdentityServiceFailsClosed(t *testing.T) {
	h := newHarness(t)
	h.auth.failWith = assertAnError{}

	response := h.request(t, http.MethodGet, "/", adaToken, nil)

	assert.Equal(t, http.StatusServiceUnavailable, response.Code)
}

type assertAnError struct{}

func (assertAnError) Error() string { return "the identity service is unreachable" }

func TestMalformedIdentifiersAreNotFound(t *testing.T) {
	h := newHarness(t)

	assert.Equal(
		t,
		http.StatusNotFound,
		h.request(t, http.MethodGet, "/posts/not%20an%20id", "", nil).Code,
	)
	assert.Equal(
		t,
		http.StatusNotFound,
		h.request(t, http.MethodGet, "/admin/users/not%20an%20id", adaToken, nil).Code,
	)
}

func TestDeletingAUserRemovesTheirRolesAndGrants(t *testing.T) {
	h := newHarness(t)
	h.authz.allow(adaRef, web.ActionUserDelete, graceRef)

	response := h.request(
		t,
		http.MethodPost,
		"/admin/users/0199bf3c-7a1e-7c2b-9f10-000000000002/delete",
		adaToken,
		url.Values{},
	)

	require.Equal(t, http.StatusSeeOther, response.Code)
	assert.Equal(t, []string{graceRef}, h.auth.deleted)
	assert.Equal(
		t,
		[]string{graceRef},
		h.authz.purged,
		"roles and grants must not outlive the account",
	)
}

// Deleting yourself from the administration pages would sign you out halfway through, and could
// remove the last administrator with nothing able to grant the role back.
func TestDeletingYourOwnAccountIsRefused(t *testing.T) {
	h := newHarness(t)
	h.authz.allow(adaRef, web.ActionUserDelete, adaRef)

	response := h.request(
		t,
		http.MethodPost,
		"/admin/users/"+adaID+"/delete",
		adaToken,
		url.Values{},
	)

	assert.Equal(t, http.StatusBadRequest, response.Code)
	assert.Empty(t, h.auth.deleted)
}
