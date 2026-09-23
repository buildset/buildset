package ui_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/buildset/buildset/auth"
	"github.com/buildset/buildset/auth/hash"
	authsqlite "github.com/buildset/buildset/auth/sqlite"
	"github.com/buildset/buildset/auth/ui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

const sessionCookieName = "ms_session"

var errAuthzUnreachable = errors.New("authz is unreachable")

type recordingHook struct {
	refs []string
	err  error
}

func (h *recordingHook) OnFirstUser(_ context.Context, userRef string) error {
	if h.err != nil {
		return h.err
	}

	h.refs = append(h.refs, userRef)

	return nil
}

type harness struct {
	server *httptest.Server
	client *http.Client
	db     *sql.DB
	hook   *recordingHook
	// registerable decides what the registration policy answers.
	registerable func(actorRef string) bool
}

func newHarness(t *testing.T) *harness {
	t.Helper()

	db, err := sql.Open("sqlite", "file:"+t.Name()+"?mode=memory&cache=shared")
	require.NoError(t, err)

	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	repository, err := authsqlite.NewRepository(t.Context(), db)
	require.NoError(t, err)

	bcryptAlgorithm, err := hash.NewBcrypt(4)
	require.NoError(t, err)

	passwords, err := hash.NewRegistry(bcryptAlgorithm)
	require.NoError(t, err)

	hook := &recordingHook{}

	service, err := auth.NewService(repository, passwords, hook, time.Hour, slog.New(slog.DiscardHandler))
	require.NoError(t, err)

	h := &harness{db: db, hook: hook, registerable: func(string) bool { return true }}

	// Who may register is decided by the composition root, not by this package, so the test
	// supplies the decision the same way.
	policy := ui.RegistrationPolicyFunc(func(_ context.Context, actorRef string) (bool, error) {
		return h.registerable(actorRef), nil
	})

	handler, err := ui.NewHandler(service, policy, ui.Config{
		SessionCookieName: sessionCookieName,
		SecureCookies:     false,
	}, slog.New(slog.DiscardHandler))
	require.NoError(t, err)

	mux := http.NewServeMux()
	handler.Register(mux)

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	jar, err := cookiejar.New(nil)
	require.NoError(t, err)

	client := &http.Client{
		Jar: jar,
		// Assert on the redirect itself rather than on wherever it points.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}

	h.server = server
	h.client = client

	return h
}

func (h *harness) get(t *testing.T, path string) *http.Response {
	t.Helper()

	response, err := h.client.Get(h.server.URL + path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = response.Body.Close() })

	return response
}

func (h *harness) post(t *testing.T, path string, form url.Values) *http.Response {
	t.Helper()

	response, err := h.client.PostForm(h.server.URL+path, form)
	require.NoError(t, err)
	t.Cleanup(func() { _ = response.Body.Close() })

	return response
}

func (h *harness) sessionCookie(t *testing.T) string {
	t.Helper()

	parsed, err := url.Parse(h.server.URL)
	require.NoError(t, err)

	for _, cookie := range h.client.Jar.Cookies(parsed) {
		if cookie.Name == sessionCookieName {
			return cookie.Value
		}
	}

	return ""
}

func (h *harness) countSessions(t *testing.T) int {
	t.Helper()

	var count int
	require.NoError(t, h.db.QueryRowContext(t.Context(), `SELECT count(*) FROM sessions`).Scan(&count))

	return count
}

func (h *harness) passwordHash(t *testing.T, username string) string {
	t.Helper()

	var stored string
	require.NoError(t, h.db.QueryRowContext(t.Context(), `SELECT password_hash FROM users WHERE username = ?`, username).Scan(&stored))

	return stored
}

func body(t *testing.T, response *http.Response) string {
	t.Helper()

	content, err := io.ReadAll(response.Body)
	require.NoError(t, err)

	return string(content)
}

func setupForm() url.Values {
	return url.Values{"username": {"ada"}, "name": {"Ada"}, "password": {"correct horse"}}
}

func TestSetupRunsOnceAndMakesTheFirstUserKnownToTheHook(t *testing.T) {
	h := newHarness(t)

	require.Equal(t, http.StatusOK, h.get(t, "/setup").StatusCode)

	response := h.post(t, "/setup", setupForm())
	require.Equal(t, http.StatusSeeOther, response.StatusCode)
	assert.Equal(t, "/", response.Header.Get("Location"))

	require.Len(t, h.hook.refs, 1)
	assert.True(t, strings.HasPrefix(h.hook.refs[0], "urn:auth:user:"), h.hook.refs[0])

	// Setup is gone for good, and says so with a 404 rather than advertising that it existed.
	assert.Equal(t, http.StatusNotFound, h.get(t, "/setup").StatusCode)
	assert.Equal(t, http.StatusNotFound, h.post(t, "/setup", setupForm()).StatusCode)
}

func TestSetupRollsBackWhenTheHookFails(t *testing.T) {
	h := newHarness(t)
	h.hook.err = errAuthzUnreachable

	assert.Equal(t, http.StatusInternalServerError, h.post(t, "/setup", setupForm()).StatusCode)

	var count int
	require.NoError(t, h.db.QueryRowContext(t.Context(), `SELECT count(*) FROM users`).Scan(&count))
	assert.Zero(t, count, "a user with no administrator role must not survive")

	// Setup stays open, so the operator can try again once the hook works.
	assert.Equal(t, http.StatusOK, h.get(t, "/setup").StatusCode)
}

func TestLoginStoresOnlyTheHashOfTheSessionToken(t *testing.T) {
	h := newHarness(t)
	require.Equal(t, http.StatusSeeOther, h.post(t, "/setup", setupForm()).StatusCode)

	token := h.sessionCookie(t)
	require.NotEmpty(t, token)

	var storedHash string
	require.NoError(t, h.db.QueryRowContext(t.Context(), `SELECT token_hash FROM sessions`).Scan(&storedHash))

	assert.NotEqual(t, token, storedHash, "the token itself must not be stored")

	sum := sha256.Sum256([]byte(token))
	assert.Equal(t, hex.EncodeToString(sum[:]), storedHash)
}

func TestLoginRejectsBadCredentialsWithoutRevealingWhichPartWasWrong(t *testing.T) {
	h := newHarness(t)
	require.Equal(t, http.StatusSeeOther, h.post(t, "/setup", setupForm()).StatusCode)

	wrongPassword := h.post(t, "/login", url.Values{"username": {"ada"}, "password": {"wrong password"}})
	require.Equal(t, http.StatusUnauthorized, wrongPassword.StatusCode)

	unknownUser := h.post(t, "/login", url.Values{"username": {"grace"}, "password": {"wrong password"}})
	require.Equal(t, http.StatusUnauthorized, unknownUser.StatusCode)

	assert.Equal(t, body(t, wrongPassword), strings.Replace(body(t, unknownUser), "grace", "ada", 1),
		"the two failures must differ only in the username echoed back")
}

func TestLogoutRemovesTheSession(t *testing.T) {
	h := newHarness(t)
	require.Equal(t, http.StatusSeeOther, h.post(t, "/setup", setupForm()).StatusCode)
	require.Equal(t, 1, h.countSessions(t))

	require.Equal(t, http.StatusSeeOther, h.post(t, "/logout", nil).StatusCode)
	assert.Zero(t, h.countSessions(t))
	assert.Empty(t, h.sessionCookie(t))
}

func TestChangePasswordRotatesTheHashAndKeepsOnlyTheCurrentSession(t *testing.T) {
	h := newHarness(t)
	require.Equal(t, http.StatusSeeOther, h.post(t, "/setup", setupForm()).StatusCode)

	// A second browser signs in as the same user.
	other := newClient(t, h)
	require.Equal(t, http.StatusSeeOther, other.post(t, "/login", url.Values{"username": {"ada"}, "password": {"correct horse"}}).StatusCode)
	require.Equal(t, 2, h.countSessions(t))

	before := h.passwordHash(t, "ada")

	wrongCurrent := h.post(t, "/password", url.Values{"current_password": {"not it"}, "new_password": {"a longer secret"}})
	require.Equal(t, http.StatusBadRequest, wrongCurrent.StatusCode)
	assert.Equal(t, before, h.passwordHash(t, "ada"))

	changed := h.post(t, "/password", url.Values{"current_password": {"correct horse"}, "new_password": {"a longer secret"}})
	require.Equal(t, http.StatusSeeOther, changed.StatusCode)

	assert.NotEqual(t, before, h.passwordHash(t, "ada"))
	assert.Equal(t, 1, h.countSessions(t), "the other browser must be signed out")
	assert.Equal(t, http.StatusOK, h.get(t, "/password").StatusCode, "this browser must stay signed in")
}

// newClient gives the same server a second, independent browser.
func newClient(t *testing.T, h *harness) *harness {
	t.Helper()

	jar, err := cookiejar.New(nil)
	require.NoError(t, err)

	return &harness{
		server:       h.server,
		db:           h.db,
		hook:         h.hook,
		registerable: h.registerable,
		client:       &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
	}
}

// With public sign-up closed, the form is not merely refused but absent, so a stranger cannot tell
// that registration exists here at all.
func TestRegistrationIsHiddenFromThoseWhoMayNotUseIt(t *testing.T) {
	h := newHarness(t)
	require.Equal(t, http.StatusSeeOther, h.post(t, "/setup", setupForm()).StatusCode)

	adminRef := h.hook.refs[0]
	h.registerable = func(actorRef string) bool { return actorRef == adminRef }

	stranger := newClient(t, h)
	assert.Equal(t, http.StatusNotFound, stranger.get(t, "/register").StatusCode)
	assert.Equal(t, http.StatusNotFound, stranger.post(t, "/register", url.Values{
		"username": {"mallory"}, "password": {"a good long secret"},
	}).StatusCode)

	assert.Equal(t, http.StatusOK, h.get(t, "/register").StatusCode)
}

// Adding somebody else's account must not swap the browser over to it.
func TestAddingAnAccountForSomeoneElseKeepsYouSignedIn(t *testing.T) {
	h := newHarness(t)
	require.Equal(t, http.StatusSeeOther, h.post(t, "/setup", setupForm()).StatusCode)

	before := h.sessionCookie(t)

	response := h.post(t, "/register", url.Values{
		"username": {"grace"}, "name": {"Grace"}, "password": {"another good secret"},
	})
	require.Equal(t, http.StatusSeeOther, response.StatusCode)
	assert.Contains(t, response.Header.Get("Location"), "/admin/users/")

	assert.Equal(t, before, h.sessionCookie(t), "the browser must still be the administrator")

	var count int
	require.NoError(t, h.db.QueryRowContext(t.Context(), `SELECT count(*) FROM users`).Scan(&count))
	assert.Equal(t, 2, count)

	// Only the account created at setup went through the first-user hook.
	assert.Len(t, h.hook.refs, 1)
}
