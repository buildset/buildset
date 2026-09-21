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
	"strconv"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/nasermirzaei89/env"
)

// healthcheckFlag turns the binary into a probe of itself. The service images have no shell, so
// this is how a container healthcheck asks whether the process inside it is ready.
const healthcheckFlag = "-healthcheck"

// Main is every main function in cmd/: load .env, catch a signal, run, report, exit.
func Main(run func(context.Context) error) {
	// A missing .env is normal: every setting has a default or comes from the real environment.
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		fmt.Fprintf(os.Stderr, "load .env: %v\n", err)
		os.Exit(1)
	}

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

// Healthcheck asks this process's own readiness probe how it is doing. It reads PORT the same way
// the server does, so the two can never disagree about where to look.
func Healthcheck() error {
	port := env.GetInt("PORT", 8080)

	client := &http.Client{Timeout: 2 * time.Second}

	response, err := client.Get("http://127.0.0.1:" + strconv.Itoa(port) + "/readyz")
	if err != nil {
		return fmt.Errorf("probe readyz: %w", err)
	}
	defer response.Body.Close()

	io.Copy(io.Discard, io.LimitReader(response.Body, 1<<10))

	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("readyz answered %d", response.StatusCode)
	}

	return nil
}
