package migrations

import "testing"

func TestLatestVersionIsDiscoveredFromEmbeddedMigrations(t *testing.T) {
	got, err := LatestVersion()
	if err != nil {
		t.Fatalf("LatestVersion() error = %v", err)
	}
	if got != 6 {
		t.Fatalf("LatestVersion() = %d, want 6", got)
	}
}
