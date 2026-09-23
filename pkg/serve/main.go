package serve

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"slices"
	"syscall"
	"time"

	"github.com/buildset/buildset/pkg/config"
	"github.com/joho/godotenv"
)

// The service images have no shell, so this is how a container healthcheck asks whether the process
// inside it is ready.
const healthcheckFlag = "-healthcheck"

var errNotReady = errors.New("service is not ready")

// Main is every main function in cmd/: load .env, catch a signal, run, report, exit.
func Main(run func(context.Context) error) {
	loadDotEnv()

	if slices.Contains(os.Args[1:], healthcheckFlag) {
		if err := Healthcheck(); err != nil {
			fmt.Fprintf(os.Stderr, "healthcheck: %v\n", err)
			os.Exit(1)
		}

		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "run: %v\n", err)
		os.Exit(1)
	}
}

// A malformed .env exits; a missing one is fine, since production sets the environment elsewhere.
func loadDotEnv() {
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		fmt.Fprintf(os.Stderr, "load .env: %v\n", err)
		os.Exit(1)
	}
}

// Healthcheck asks this process's own readiness probe, loading the server settings the same way the
// server does so the two cannot disagree about where to look.
func Healthcheck() error {
	port, err := config.LoadServer().ResolvePort()
	if err != nil {
		return err
	}

	client := &http.Client{Timeout: 2 * time.Second}

	response, err := client.Get("http://127.0.0.1:" + port + "/readyz")
	if err != nil {
		return fmt.Errorf("probe readyz: %w", err)
	}
	defer func() { _ = response.Body.Close() }()

	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<10))

	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: readyz answered %d", errNotReady, response.StatusCode)
	}

	return nil
}
