// Package migrations embeds the SQL migration files so a single binary can
// apply them (no separate migrate CLI needed in containers or CI).
package migrations

import "embed"

// FS contains every *.sql migration file in this directory.
//
//go:embed *.sql
var FS embed.FS
