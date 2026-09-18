package config

import (
	"strings"
	"testing"
)

func categorizationEnv(overrides map[string]string) func(string) string {
	base := map[string]string{"DATABASE_URL": "postgres://finance:password@localhost:5432/finance?sslmode=disable", "FINANCE_JEV_TOKEN": "token-value"}
	for key, value := range overrides {
		base[key] = value
	}
	return func(key string) string { return base[key] }
}

func TestLoadCategorizationDefaultsAndValidation(t *testing.T) {
	cfg, err := LoadCategorizationFrom(categorizationEnv(nil))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.JevEndpoint != "https://api.typesafe.ai/v1/systemone" || cfg.JevModel != "jev-latest" || cfg.CategorizeThreshold != .85 || cfg.CategorizeBatchLimit != 100 {
		t.Fatalf("defaults = %#v", cfg)
	}
	for _, tc := range []struct{ key, value string }{{"FINANCE_CATEGORIZE_THRESHOLD", "1.1"}, {"FINANCE_CATEGORIZE_THRESHOLD", "nope"}, {"FINANCE_CATEGORIZE_BATCH_LIMIT", "0"}, {"FINANCE_CATEGORIZE_BATCH_LIMIT", "nope"}} {
		_, err := LoadCategorizationFrom(categorizationEnv(map[string]string{tc.key: tc.value}))
		if err == nil {
			t.Fatalf("%s=%q accepted", tc.key, tc.value)
		}
	}
}

func TestLoadCategorizationRequiresJevTokenWithoutLeakingValue(t *testing.T) {
	const token = "must-not-appear"
	_, err := LoadCategorizationFrom(categorizationEnv(map[string]string{"FINANCE_JEV_TOKEN": " "}))
	if err == nil || !strings.Contains(err.Error(), "FINANCE_JEV_TOKEN is required") || strings.Contains(err.Error(), token) {
		t.Fatalf("error = %v", err)
	}
}
