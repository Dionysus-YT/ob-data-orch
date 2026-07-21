// Package migrations exposes immutable, versioned SQLite migrations.
package migrations

import "embed"

// Files contains the forward-only migration set shipped with the application.
//
//go:embed *.sql
var Files embed.FS
