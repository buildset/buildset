package app

import (
	"context"
	"log/slog"
	"net/http"
	"time"
	"uuid"
)

type requestIDKey struct{}

// RequestIDHeader carries the identifier back to the client, so a report of a failure can be tied
// to the lines this process logged about it.
const RequestIDHeader = "X-Request-Id"

// withRequestID tags each request so every log line about it can be found together.
//
// TODO: an inbound X-Request-Id is ignored. Honouring one needs a list of trusted proxies first,
// otherwise any client can choose what its requests are filed under.
func withRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := uuid.NewV7().String()
		w.Header().Set(RequestIDHeader, id)

		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey{}, id)))
	})
}

func requestIDFromContext(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(requestIDKey{}).(string)

	return id, ok
}

// recoverPanics keeps one bad handler from taking the process down, and makes sure the client gets
// a response rather than a dropped connection.
func recoverPanics(logger *slog.Logger, next http.Handler) http.Handler {
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

func logRequests(logger *slog.Logger, next http.Handler) http.Handler {
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
