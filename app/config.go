package app

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/buildset/buildset/pkg/config"
	"github.com/buildset/buildset/pkg/storage"
	"github.com/nasermirzaei89/env"
)

// SessionCookieName is fixed rather than configurable. Both the service that sets the cookie and
// the one that reads it are given this value, so neither hard-codes the other's choice.
const SessionCookieName = config.SessionCookieName

// The storage backends, re-exported so a caller building a Config need not import two packages.
const (
	DriverSQLite   = storage.DriverSQLite
	DriverPostgres = storage.DriverPostgres
)

// DatabaseConfig is this binary's storage settings.
type DatabaseConfig = storage.Config

type Config struct {
	config.Server

	LogLevel          slog.Level
	LogJSON           bool
	SessionCookieName string
	SecureCookies     bool
	SiteTitle         string
	Database          DatabaseConfig
	Auth              AuthConfig
}

type AuthConfig struct {
	BcryptCost       int
	SessionTTL       time.Duration
	RegistrationOpen bool
}

func LoadConfig() (*Config, error) {
	log, err := config.LoadLog()
	if err != nil {
		return nil, err
	}

	cookie := config.LoadCookie()

	cfg := &Config{
		Server:            config.LoadServer(),
		LogLevel:          log.Level,
		LogJSON:           log.JSON,
		SessionCookieName: cookie.Name,
		SecureCookies:     cookie.Secure,
		SiteTitle:         env.GetString("SITE_TITLE", "Blog"),
		Database:          storage.Load(),
		Auth:              LoadAuthConfig(),
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// LoadAuthConfig is shared with the identity binary, which reads the same variables.
func LoadAuthConfig() AuthConfig {
	return AuthConfig{
		BcryptCost:       env.GetInt("AUTH_BCRYPT_COST", 12),
		SessionTTL:       env.GetDuration("AUTH_SESSION_TTL", 14*24*time.Hour),
		RegistrationOpen: env.GetBool("AUTH_REGISTRATION_OPEN", false),
	}
}

func (c AuthConfig) Validate() error {
	// bcrypt rejects costs outside [4, 31]; bounding here turns a per-login failure into a boot failure.
	if c.BcryptCost < 10 || c.BcryptCost > 31 {
		return fmt.Errorf("AUTH_BCRYPT_COST must be between 10 and 31, got %d", c.BcryptCost)
	}

	if c.SessionTTL <= 0 {
		return fmt.Errorf("AUTH_SESSION_TTL must be positive, got %s", c.SessionTTL)
	}

	return nil
}

func (c *Config) Validate() error {
	if err := c.Server.Validate(); err != nil {
		return err
	}

	if err := c.Database.Validate(); err != nil {
		return err
	}

	return c.Auth.Validate()
}
