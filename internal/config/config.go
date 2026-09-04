// Package config loads and validates the process environment.
package config

import (
	"fmt"
	"net/url"
	"os"
	"strings"
)

// Config contains validated application configuration.
type Config struct {
	DatabaseURL string
	APIToken    string
	AppOrigin   string
}

// Load reads the required environment variables.
func Load() (Config, error) { return LoadFrom(os.Getenv) }

// LoadDatabase reads and validates only the database setting. Migrations do
// not need the HTTP API token and must be independently runnable.
func LoadDatabase() (Config, error) { return LoadDatabaseFrom(os.Getenv) }

// LoadDatabaseFrom is LoadDatabase with an injectable environment reader.
func LoadDatabaseFrom(getenv func(string) string) (Config, error) {
	databaseURL := strings.TrimSpace(getenv("DATABASE_URL"))
	if databaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}
	if err := validateDatabaseURL(databaseURL); err != nil {
		return Config{}, err
	}
	return Config{DatabaseURL: databaseURL}, nil
}

// LoadFrom is Load with an injectable environment reader, useful for tests.
func LoadFrom(getenv func(string) string) (Config, error) {
	cfg, err := LoadDatabaseFrom(getenv)
	if err != nil {
		return Config{}, err
	}

	token := strings.TrimSpace(getenv("FINANCE_API_TOKEN"))
	if token == "" {
		return Config{}, fmt.Errorf("FINANCE_API_TOKEN is required")
	}
	origin := strings.TrimSpace(getenv("APP_ORIGIN"))
	if origin == "" {
		origin = "http://localhost:8080"
	}
	if err := validateOrigin(origin); err != nil {
		return Config{}, err
	}
	cfg.APIToken = token
	cfg.AppOrigin = origin
	return cfg, nil
}

func validateOrigin(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return fmt.Errorf("APP_ORIGIN must be an exact http(s) origin")
	}
	return nil
}

func validateDatabaseURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "postgres" && u.Scheme != "postgresql" || u.Host == "" || u.Path == "" || u.Path == "/" {
		return fmt.Errorf("DATABASE_URL must be a valid postgres connection URL")
	}
	if u.User == nil || u.User.Username() == "" {
		return fmt.Errorf("DATABASE_URL must include a database user")
	}
	return nil
}
