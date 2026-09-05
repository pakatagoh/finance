// Package migrations contains the SQL migrations shipped with Finance.
package migrations

import (
	"embed"
	"fmt"
	"strconv"
	"strings"
)

// FS is the read-only filesystem embedded in the Finance binary.
// Migration files are intentionally rooted at the package directory.
//
//go:embed *.sql
var FS embed.FS

// LatestVersion discovers the highest Goose migration version embedded in FS.
func LatestVersion() (int64, error) {
	entries, err := FS.ReadDir(".")
	if err != nil {
		return 0, fmt.Errorf("read embedded migrations: %w", err)
	}

	var latest int64
	found := false
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		version, err := migrationVersion(entry.Name())
		if err != nil {
			return 0, err
		}
		if !found || version > latest {
			latest = version
			found = true
		}
	}
	if !found {
		return 0, fmt.Errorf("no embedded SQL migrations found")
	}
	return latest, nil
}

// MustLatestVersion returns the latest embedded migration version or panics if
// the application contains an invalid migration set.
func MustLatestVersion() int64 {
	version, err := LatestVersion()
	if err != nil {
		panic(err)
	}
	return version
}

func migrationVersion(name string) (int64, error) {
	separator := strings.IndexByte(name, '_')
	if separator <= 0 {
		return 0, fmt.Errorf("invalid migration filename %q: want NNNNN_name.sql", name)
	}
	version, err := strconv.ParseInt(name[:separator], 10, 64)
	if err != nil || version < 1 {
		return 0, fmt.Errorf("invalid migration filename %q: version must be a positive integer", name)
	}
	return version, nil
}
