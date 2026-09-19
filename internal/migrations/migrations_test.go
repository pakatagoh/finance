package migrations

import (
	"strings"
	"testing"
)

func TestLatestVersionIsDiscoveredFromEmbeddedMigrations(t *testing.T) {
	got, err := LatestVersion()
	if err != nil {
		t.Fatalf("LatestVersion() error = %v", err)
	}
	if got != 8 {
		t.Fatalf("LatestVersion() = %d, want 8", got)
	}

	migration, err := FS.ReadFile("00007_categorization.sql")
	if err != nil {
		t.Fatalf("read categorization migration: %v", err)
	}
	text := string(migration)
	for _, want := range []string{"category_source", "category_confidence", "category_model", "category_categorized_at", "category_mappings", "UNIQUE (normalized_counterparty, kind, direction, currency)"} {
		if !strings.Contains(text, want) {
			t.Errorf("migration missing %q", want)
		}
	}
}
