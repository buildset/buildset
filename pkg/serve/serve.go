// Package serve is the part of running a service that has nothing to do with what the service
// does: a logger, a listener with sane timeouts, request tagging, panic recovery, request logging,
// liveness and readiness, and a shutdown that finishes what it started.
//
// It exists so that splitting a service into its own binary does not mean copying this, which is
// how four binaries drift apart.
package serve

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// Options describes one process.
type Options struct {
	// Name appears in the start-up and shutdown lines: "auth", "web", "blog".
	Name string

	Address         string
	ShutdownTimeout time.Duration
	Logger          *slog.Logger

	// Routes are the service's own paths. The health probes are added here so no service has to
	// remember them and so they answer identically in every binary.
	Routes http.Handler

	// Ready backs /readyz. Nil means the process is ready as soon as it is listening, which is the
	// right answer for a binary with no storage of its own.
	//
	// It must not check a dependency. If web's readiness followed auth's, restarting auth would
	// mark web unready too and the gateway would have nothing to route to, turning one service's
	// blip into a full outage.
	Ready func(ctx context.Context) error

	// CrossOrigin applies http.CrossOriginProtection. True for a binary a browser reaches, false
	// for a JSON API on a private network that no browser ever sees.
	CrossOrigin bool

	// TrustRequestID honours an inbound X-Request-Id. True only where the caller is a sibling
	// service, never on anything reachable through the gateway.
	TrustRequestID bool

	// Background are goroutines tied to the server's lifetime, such as the session sweeper.
	Background []func(context.Context)
}

// readyTimeout bounds a readiness probe, so a wedged database makes the probe fail rather than
// pile up connections.
const readyTimeout = 2 * time.Second

// Handler assembles the routes, the health probes and the middleware into the handler this process
// serves. It is separated from Run so a test can drive it without a listener.
func Handler(opts Options) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writePlain(w, http.StatusOK, "ok")
	})

	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if opts.Ready == nil {
			writePlain(w, http.StatusOK, "ready")

			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), readyTimeout)
		defer cancel()

		if err := opts.Ready(ctx); err != nil {
			writePlain(w, http.StatusServiceUnavailable, "not ready")

			return
		}

		writePlain(w, http.StatusOK, "ready")
	})

	if opts.Routes != nil {
		mux.Handle("/", opts.Routes)
	}

	var handler http.Handler = mux

	if opts.CrossOrigin {
		// Rejects cross-origin state-changing requests using Sec-Fetch-Site, falling back to Origin.
		// TODO: requests carrying neither header are allowed through. Combined with SameSite=Lax that
		// leaves only pre-2023 browsers exposed; add session-bound form tokens if those must be supported.
		handler = http.NewCrossOriginProtection().Handler(handler)
	}

	// Request tagging, logging and panic recovery are a property of the process rather than of any
	// service in it. The identifier is applied first so everything below can log it.
	return WithRequestID(opts.TrustRequestID, RecoverPanics(opts.Logger, LogRequests(opts.Logger, handler)))
}

// Run serves until the context is cancelled, then shuts down gracefully.
func Run(ctx context.Context, opts Options) error {
	logger := opts.Logger

	for _, background := range opts.Background {
		go background(ctx)
	}

	server := &http.Server{
		Addr:              opts.Address,
		Handler:           Handler(opts),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       2 * time.Minute,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelWarn),
	}

	errs := make(chan error, 1)

	go func() {
		logger.Info("http server listening",
			slog.String("service", opts.Name),
			slog.String("address", opts.Address),
		)

		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errs <- fmt.Errorf("listen and serve: %w", err)

			return
		}

		errs <- nil
	}()

	select {
	case err := <-errs:
		return err
	case <-ctx.Done():
		logger.Info("shutdown signal received", slog.String("service", opts.Name))
	}

	// The shutdown deadline must survive the cancelled context that triggered it.
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), opts.ShutdownTimeout)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown http server: %w", err)
	}

	return <-errs
}

func writePlain(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	w.Write([]byte(body))
}
