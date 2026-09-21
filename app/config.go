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

// Driver names the storage backend. Both are supported everywhere; SQLite is the default because
// it needs nothing to be running.
const (
	DriverSQLite   = "sqlite"
	DriverPostgres = "postgres"
)

type DatabaseConfig struct {
	// Driver is DriverSQLite or DriverPostgres.
	Driver string
	// Path is the SQLite file. The postgres driver ignores it.
	Path string
	// DSN is the Postgres connection string. The sqlite driver ignores it.
	DSN string
	// MaxOpenConns bounds each Postgres pool. SQLite is always one connection.
	MaxOpenConns int
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
			Driver:       env.GetString("DATABASE_DRIVER", DriverSQLite),
			Path:         env.GetString("DATABASE_PATH", "buildset.db"),
			DSN:          env.GetString("DATABASE_DSN", ""),
			MaxOpenConns: env.GetInt("DATABASE_MAX_OPEN_CONNS", 10),
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

func (c DatabaseConfig) Validate() error {
	switch c.Driver {
	case DriverSQLite:
		if c.Path == "" {
			return fmt.Errorf("DATABASE_PATH must not be empty")
		}
	case DriverPostgres:
		if c.DSN == "" {
			return fmt.Errorf("DATABASE_DSN must not be empty when DATABASE_DRIVER is %q", DriverPostgres)
		}

		if c.MaxOpenConns < 1 {
			return fmt.Errorf("DATABASE_MAX_OPEN_CONNS must be positive, got %d", c.MaxOpenConns)
		}
	default:
		return fmt.Errorf("DATABASE_DRIVER must be %q or %q, got %q", DriverSQLite, DriverPostgres, c.Driver)
	}

	return nil
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

	if err := c.Database.Validate(); err != nil {
		return err
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
