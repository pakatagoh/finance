// Package migrations contains the SQL migrations shipped with Finance.
package migrations

import "embed"

// FS is the read-only filesystem embedded in the Finance binary.
// Migration files are intentionally rooted at the package directory.
//
//go:embed *.sql
var FS embed.FS

// LatestVersion is the highest embedded Goose migration version.
const LatestVersion int64 = 3
