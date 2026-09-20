package app

import (
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"time"

	"github.com/nasermirzaei89/env"
)

// SessionCookieName is fixed rather than configurable. Both the service that sets the cookie and
// the one that reads it are given this value, so neither hard-codes the other's choice.
const SessionCookieName = "ms_session"

type Config struct {
	Host              string
	Port              int
	LogLevel          slog.Level
	LogJSON           bool
	SessionCookieName string
	SecureCookies     bool
	SiteTitle         string
	ShutdownTimeout   time.Duration
	Database          DatabaseConfig
	Auth              AuthConfig
}

type DatabaseConfig struct {
	Path string
}

type AuthConfig struct {
	BcryptCost       int
	SessionTTL       time.Duration
	RegistrationOpen bool
}

func LoadConfig() (*Config, error) {
	var level slog.Level
	if err := level.UnmarshalText([]byte(env.GetString("LOG_LEVEL", "info"))); err != nil {
		return nil, fmt.Errorf("parse LOG_LEVEL: %w", err)
	}

	cfg := &Config{
		Host:              env.GetString("HOST", ""),
		Port:              env.GetInt("PORT", 8080),
		LogLevel:          level,
		LogJSON:           env.GetBool("LOG_JSON", false),
		SessionCookieName: SessionCookieName,
		SecureCookies:     env.GetBool("SECURE_COOKIES", true),
		SiteTitle:         env.GetString("SITE_TITLE", "Blog"),
		ShutdownTimeout:   env.GetDuration("SHUTDOWN_TIMEOUT", 15*time.Second),
		Database: DatabaseConfig{
			Path: env.GetString("DATABASE_PATH", "ms.db"),
		},
		Auth: AuthConfig{
			BcryptCost:       env.GetInt("AUTH_BCRYPT_COST", 12),
			SessionTTL:       env.GetDuration("AUTH_SESSION_TTL", 14*24*time.Hour),
			RegistrationOpen: env.GetBool("AUTH_REGISTRATION_OPEN", false),
		},
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Address is what the HTTP server listens on. An empty host means every interface.
func (c *Config) Address() string {
	return net.JoinHostPort(c.Host, strconv.Itoa(c.Port))
}

func (c *Config) Validate() error {
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("PORT must be between 1 and 65535, got %d", c.Port)
	}

	if c.ShutdownTimeout <= 0 {
		return fmt.Errorf("SHUTDOWN_TIMEOUT must be positive, got %s", c.ShutdownTimeout)
	}

	if c.Database.Path == "" {
		return fmt.Errorf("DATABASE_PATH must not be empty")
	}

	// bcrypt rejects costs outside [4, 31]; bounding here turns a per-login failure into a boot failure.
	if c.Auth.BcryptCost < 10 || c.Auth.BcryptCost > 31 {
		return fmt.Errorf("AUTH_BCRYPT_COST must be between 10 and 31, got %d", c.Auth.BcryptCost)
	}

	if c.Auth.SessionTTL <= 0 {
		return fmt.Errorf("AUTH_SESSION_TTL must be positive, got %s", c.Auth.SessionTTL)
	}

	return nil
}
