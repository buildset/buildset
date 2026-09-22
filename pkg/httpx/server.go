package httpx

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
)

// maxRequestBytes bounds a request body, so a caller cannot use one to exhaust a service.
const maxRequestBytes = 1 << 20

// DecodeJSON reads a JSON request body. A body it cannot read is the caller's fault, so it answers
// with an envelope rather than a failure, and reports false to stop the handler.
func DecodeJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	decoder := json.NewDecoder(io.LimitReader(r.Body, maxRequestBytes))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(target); err != nil {
		// The error is not echoed back: a decode failure can quote the body, and one of these
		// bodies carries a live session token.
		WriteError(w, CodeInvalidInput, "That request could not be read.")

		return false
	}

	return true
}

// WriteJSON answers a successful call.
func WriteJSON(w http.ResponseWriter, value any) {
	body, err := json.Marshal(value)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)

		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	w.Write(body)
}

// WriteError answers a call the caller got wrong. message is written for a visitor, because it is
// what the site will show.
func WriteError(w http.ResponseWriter, code Code, message string) {
	body, err := json.Marshal(Envelope{Code: code, Message: message})
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)

		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code.Status())
	w.Write(body)
}

// WriteInternal answers a call this service could not complete. The cause is logged and not sent,
// so a client sees a failure it must not mistake for an answer.
func WriteInternal(ctx context.Context, w http.ResponseWriter, logger *slog.Logger, operation string, err error) {
	logger.ErrorContext(ctx, operation, slog.Any("error", err))

	http.Error(w, fmt.Sprintf("%s failed", operation), http.StatusInternalServerError)
}
