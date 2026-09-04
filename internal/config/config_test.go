package config

import (
	"strings"
	"testing"
)

func TestLoadFromRequiresDatabaseURLAndToken(t *testing.T) {
	_, err := LoadFrom(func(key string) string {
		if key == "FINANCE_API_TOKEN" {
			return "secret"
		}
		return ""
	})
	if err == nil || !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Fatalf("expected missing DATABASE_URL error, got %v", err)
	}
}

func TestLoadFromValidatesDatabaseURLWithoutLeakingCredentials(t *testing.T) {
	const secret = "super-secret-password"
	cfg, err := LoadFrom(func(key string) string {
		switch key {
		case "DATABASE_URL":
			return "postgres://finance:" + secret + "@localhost:5432/finance?sslmode=disable"
		case "FINANCE_API_TOKEN":
			return "token"
		default:
			return ""
		}
	})
	if err != nil {
		t.Fatalf("LoadFrom() error = %v", err)
	}
	if cfg.DatabaseURL == "" || cfg.APIToken == "" {
		t.Fatal("expected loaded configuration")
	}
}

func TestLoadFromRejectsNonPostgresURL(t *testing.T) {
	_, err := LoadFrom(func(key string) string {
		if key == "DATABASE_URL" {
			return "sqlite://finance"
		}
		if key == "FINANCE_API_TOKEN" {
			return "token"
		}
		return ""
	})
	if err == nil || !strings.Contains(err.Error(), "postgres") {
		t.Fatalf("expected postgres URL error, got %v", err)
	}
}
