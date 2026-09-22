// Package config holds the settings blocks that more than one binary needs, so five composition
// roots share one definition of what PORT or DATABASE_DSN means instead of five that drift.
//
// A binary embeds the blocks it needs and adds its own fields; nothing here knows the whole set.
package config

import (
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/nasermirzaei89/env"
)

// SessionCookieName is fixed rather than configurable. Both the service that sets the cookie and
// the one that reads it are given this value, so neither hard-codes the other's choice.
const SessionCookieName = "ms_session"

// Server is where and how a process listens. Port stays the string the environment gave us: that
// is what net.Listen wants, and it lets PORT name a service ("http") as well as a number.
type Server struct {
	Host            string
	Port            string
	ShutdownTimeout time.Duration
}

func LoadServer() Server {
	return Server{
		Host:            env.GetString("HOST", ""),
		Port:            env.GetString("PORT", "8080"),
		ShutdownTimeout: env.GetDuration("SHUTDOWN_TIMEOUT", 15*time.Second),
	}
}

// Address is what the HTTP server listens on. An empty host means every interface.
func (s Server) Address() string {
	return net.JoinHostPort(s.Host, s.Port)
}

// ResolvePort returns Port as a decimal string, resolving a service name if PORT held one. A URL
// authority may not contain a service name, so anything building one needs this rather than Port.
func (s Server) ResolvePort() (string, error) {
	port, err := net.LookupPort("tcp", s.Port)
	if err != nil {
		return "", fmt.Errorf("resolve PORT %q: %w", s.Port, err)
	}

	return strconv.Itoa(port), nil
}

func (s Server) Validate() error {
	if _, err := s.ResolvePort(); err != nil {
		return err
	}

	if s.ShutdownTimeout <= 0 {
		return fmt.Errorf("SHUTDOWN_TIMEOUT must be positive, got %s", s.ShutdownTimeout)
	}

	return nil
}

// Log is how a process writes its logs.
type Log struct {
	Level slog.Level
	JSON  bool
}

func LoadLog() (Log, error) {
	var level slog.Level
	if err := level.UnmarshalText([]byte(env.GetString("LOG_LEVEL", "info"))); err != nil {
		return Log{}, fmt.Errorf("parse LOG_LEVEL: %w", err)
	}

	return Log{Level: level, JSON: env.GetBool("LOG_JSON", false)}, nil
}

// Cookie is the session cookie contract, shared by the binary that sets it and the one that reads
// it. They must agree, or a signed-in visitor looks anonymous to half the system.
type Cookie struct {
	Name   string
	Secure bool
}

func LoadCookie() Cookie {
	return Cookie{
		Name:   SessionCookieName,
		Secure: env.GetBool("SECURE_COOKIES", true),
	}
}

// LoadURL reads the base URL of a dependency. A missing or malformed value is a start-up failure
// rather than a 500 on a visitor's first click.
func LoadURL(key string) (string, error) {
	raw := env.GetString(key, "")
	if raw == "" {
		return "", fmt.Errorf("%s must be set", key)
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("parse %s: %w", key, err)
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("%s must be an http or https URL, got %q", key, raw)
	}

	if parsed.Host == "" {
		return "", fmt.Errorf("%s must include a host, got %q", key, raw)
	}

	// A trailing slash would double up when a path is appended.
	return strings.TrimRight(raw, "/"), nil
}
