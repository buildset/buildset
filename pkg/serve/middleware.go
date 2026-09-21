package serve

import (
	"log/slog"
	"net/http"
	"time"
	"uuid"

	"github.com/buildset/buildset/pkg/reqid"
)

// WithRequestID tags each request so every log line about it can be found together.
//
// An inbound identifier is honoured only when trustInbound is set, which is right for an internal
// API a sibling service calls and wrong for anything a browser can reach: otherwise any client
// could choose what its requests are filed under, or collide with someone else's.
func WithRequestID(trustInbound bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := ""
		if trustInbound {
			id = r.Header.Get(reqid.Header)
		}

		if !validRequestID(id) {
			id = uuid.NewV7().String()
		}

		w.Header().Set(reqid.Header, id)

		next.ServeHTTP(w, r.WithContext(reqid.NewContext(r.Context(), id)))
	})
}

// validRequestID keeps a forwarded identifier from carrying anything that would corrupt a log line
// or a response header.
func validRequestID(id string) bool {
	if len(id) == 0 || len(id) > 64 {
		return false
	}

	for _, r := range id {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			continue
		}

		return false
	}

	return true
}

// RecoverPanics keeps one bad handler from taking the process down, and makes sure the client gets
// a response rather than a dropped connection.
func RecoverPanics(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			recovered := recover()
			if recovered == nil {
				return
			}

			logger.ErrorContext(r.Context(), "handler panicked",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Any("panic", recovered),
			)

			// If the handler already started writing, the status is long gone and all we can do is
			// stop. Writing again would corrupt the response.
			http.Error(w, "internal server error", http.StatusInternalServerError)
		}()

		next.ServeHTTP(w, r)
	})
}

// LogRequests records one line per request.
//
// It logs the method, the path, the status and the duration, and nothing else. That is deliberate:
// the identity service takes a live session token in a request body, and a middleware that grew a
// habit of logging bodies or headers would put credentials in the log of every service that
// forwards one. Add a field here only after checking what can reach it.
func LogRequests(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(recorder, r)

		logger.InfoContext(r.Context(), "request",
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", recorder.status),
			slog.Duration("duration", time.Since(start)),
		)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (s *statusRecorder) WriteHeader(status int) {
	if s.wroteHeader {
		return
	}

	s.status = status
	s.wroteHeader = true
	s.ResponseWriter.WriteHeader(status)
}
