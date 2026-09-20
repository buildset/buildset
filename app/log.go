package app

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/SladkyCitron/slogcolor"
)

func NewLogger(cfg *Config) *slog.Logger {
	opts := &slog.HandlerOptions{Level: cfg.LogLevel}

	var handler slog.Handler
	if cfg.LogJSON {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slogcolor.NewHandler(os.Stdout, &slogcolor.Options{
			Level:      cfg.LogLevel,
			TimeFormat: time.RFC3339,
			// Colour is an escape sequence that means nothing in a file or a log collector, so it
			// is only used when the output is a terminal.
			NoColor: !isTerminal(os.Stdout),
		})
	}

	return slog.New(requestIDHandler{Handler: handler})
}

// requestIDHandler copies the request identifier out of the context onto every record. Services
// log through the same logger and never learn that the field exists, so a line from auth and a
// line from web about one request carry the same identifier.
type requestIDHandler struct {
	slog.Handler
}

func (h requestIDHandler) Handle(ctx context.Context, record slog.Record) error {
	if id, ok := requestIDFromContext(ctx); ok {
		record.AddAttrs(slog.String("request_id", id))
	}

	return h.Handler.Handle(ctx, record)
}

func (h requestIDHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return requestIDHandler{Handler: h.Handler.WithAttrs(attrs)}
}

func (h requestIDHandler) WithGroup(name string) slog.Handler {
	return requestIDHandler{Handler: h.Handler.WithGroup(name)}
}

func isTerminal(file *os.File) bool {
	info, err := file.Stat()
	if err != nil {
		return false
	}

	return info.Mode()&os.ModeCharDevice != 0
}
