// Package migrations exposes the SQL migration files as an embed.FS so that
// goose-based runners (production startup, tests) can apply them without
// shelling out to the goose CLI.
package migrations

import (
	"embed"
)

// FS embeds the *.sql migration files in this directory so that goose-based
// runners can apply them without shelling out to the goose CLI.
//
//go:embed *.sql
var FS embed.FS
