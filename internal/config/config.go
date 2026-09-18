// Package config loads and validates the process environment.
package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

// Config contains validated application configuration.
type Config struct {
	DatabaseURL          string
	APIToken             string
	AppOrigin            string
	JevEndpoint          string
	JevModel             string
	JevToken             string
	CategorizeThreshold  float64
	CategorizeBatchLimit int
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

func LoadCategorization() (Config, error) { return LoadCategorizationFrom(os.Getenv) }

func LoadCategorizationFrom(getenv func(string) string) (Config, error) {
	cfg, err := LoadDatabaseFrom(getenv)
	if err != nil {
		return Config{}, err
	}
	cfg.JevEndpoint = strings.TrimSpace(getenv("FINANCE_JEV_ENDPOINT"))
	if cfg.JevEndpoint == "" {
		cfg.JevEndpoint = "https://api.typesafe.ai/v1/systemone"
	}
	cfg.JevModel = strings.TrimSpace(getenv("FINANCE_JEV_MODEL"))
	if cfg.JevModel == "" {
		cfg.JevModel = "jev-latest"
	}
	cfg.JevToken = strings.TrimSpace(getenv("FINANCE_JEV_TOKEN"))
	if cfg.JevToken == "" {
		return Config{}, fmt.Errorf("FINANCE_JEV_TOKEN is required")
	}
	cfg.CategorizeThreshold = 0.85
	if raw := strings.TrimSpace(getenv("FINANCE_CATEGORIZE_THRESHOLD")); raw != "" {
		cfg.CategorizeThreshold, err = strconv.ParseFloat(raw, 64)
		if err != nil || cfg.CategorizeThreshold < 0 || cfg.CategorizeThreshold > 1 {
			return Config{}, fmt.Errorf("FINANCE_CATEGORIZE_THRESHOLD must be between 0 and 1")
		}
	}
	cfg.CategorizeBatchLimit = 100
	if raw := strings.TrimSpace(getenv("FINANCE_CATEGORIZE_BATCH_LIMIT")); raw != "" {
		cfg.CategorizeBatchLimit, err = strconv.Atoi(raw)
		if err != nil || cfg.CategorizeBatchLimit <= 0 {
			return Config{}, fmt.Errorf("FINANCE_CATEGORIZE_BATCH_LIMIT must be positive")
		}
	}
	return cfg, nil
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
