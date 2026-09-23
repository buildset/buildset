// Package tests drives the whole application through a real browser. Everything below the browser
// is the real thing: the real handler, the real services, a real SQLite file.
package tests

import (
	"context"
	"fmt"
	"log/slog"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/buildset/buildset/app"
	"github.com/nasermirzaei89/env"
	"github.com/playwright-community/playwright-go"
	"github.com/stretchr/testify/require"
)

type harness struct {
	url     string
	browser playwright.Browser
}

// The browser is downloaded at most once per test binary, and only when asked to.
var installOnce sync.Once

// installBrowserEnvVar opts in to downloading the browser during the test run.
const installBrowserEnvVar = "E2E_INSTALL_BROWSER"

// startPlaywright returns a running driver, or the reason these tests cannot run. The browser is
// around 200 MB, so CI should install and cache it as a step of its own; set E2E_INSTALL_BROWSER=1
// to have the tests download it instead.
func startPlaywright() (*playwright.Playwright, error) {
	pw, err := playwright.Run()
	if err == nil {
		return pw, nil
	}

	if !env.GetBool(installBrowserEnvVar, false) {
		return nil, fmt.Errorf("%w (set %s=1 to download it, or install it beforehand, see the README)", err, installBrowserEnvVar)
	}

	var installErr error

	installOnce.Do(func() {
		installErr = playwright.Install(&playwright.RunOptions{Browsers: []string{"chromium"}})
	})

	if installErr != nil {
		return nil, fmt.Errorf("install browser: %w", installErr)
	}

	return playwright.Run()
}

// newBrowser starts a browser, or skips the test when there is none to start.
func newBrowser(t *testing.T) playwright.Browser {
	t.Helper()

	pw, err := startPlaywright()
	if err != nil {
		t.Skipf("playwright is not available: %v", err)
	}

	t.Cleanup(func() { _ = pw.Stop() })

	browser, err := pw.Chromium.Launch()
	if err != nil {
		t.Skipf("chromium is not installed: %v", err)
	}

	t.Cleanup(func() { _ = browser.Close() })

	return browser
}

// newMonoHarness boots the single binary on a temporary database and opens a browser against it.
func newMonoHarness(t *testing.T) *harness {
	t.Helper()

	browser := newBrowser(t)

	config := &app.Config{
		SessionCookieName: app.SessionCookieName,
		// The test server speaks plain HTTP, so a Secure cookie would never come back.
		SecureCookies: false,
		SiteTitle:     "Test Blog",
		Database: app.DatabaseConfig{
			Driver: app.DriverSQLite,
			Path:   filepath.Join(t.TempDir(), "test.db"),
		},
		Auth: app.AuthConfig{
			// The lowest cost bcrypt accepts, because these tests sign in repeatedly.
			BcryptCost:       10,
			SessionTTL:       time.Hour,
			RegistrationOpen: true,
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))

	application, err := app.New(context.Background(), config, logger)
	require.NoError(t, err)
	t.Cleanup(func() { _ = application.Close() })

	server := httptest.NewServer(application.Handler())
	t.Cleanup(server.Close)

	return &harness{url: server.URL, browser: browser}
}

// topologyEnvVar chooses which arrangements the scenarios run against.
const topologyEnvVar = "E2E_TOPOLOGY"

// forEachTopology runs a scenario against the single binary and against the four services behind a
// gateway. The claim a split makes is that it behaves the same way, and one set of tests against
// both is what checks it.
func forEachTopology(t *testing.T, scenario func(*testing.T, *harness)) {
	t.Helper()

	topology := env.GetString(topologyEnvVar, "both")

	if topology == "mono" || topology == "both" {
		t.Run("mono", func(t *testing.T) { scenario(t, newMonoHarness(t)) })
	}

	if topology == "split" || topology == "both" {
		t.Run("split", func(t *testing.T) { scenario(t, newSplitHarness(t)) })
	}
}

// newPage opens a fresh browser context, which is a browser with its own cookie jar.
func (h *harness) newPage(t *testing.T) playwright.Page {
	t.Helper()

	browserContext, err := h.browser.NewContext()
	require.NoError(t, err)
	t.Cleanup(func() { _ = browserContext.Close() })

	page, err := browserContext.NewPage()
	require.NoError(t, err)

	return page
}

func (h *harness) open(t *testing.T, page playwright.Page, path string) playwright.Response {
	t.Helper()

	response, err := page.Goto(h.url + path)
	require.NoError(t, err)

	return response
}

func fill(t *testing.T, page playwright.Page, label, value string) {
	t.Helper()

	require.NoError(t, page.GetByLabel(label).Fill(value))
}

func click(t *testing.T, page playwright.Page, name string) {
	t.Helper()

	require.NoError(t, page.GetByRole("button", playwright.PageGetByRoleOptions{Name: name}).Click())
}
