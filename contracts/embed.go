// Package contracts exposes versioned protocol contracts for validation.
package contracts

import "embed"

// Files contains the API contracts shipped with the application.
//
//go:embed *.json
var Files embed.FS
