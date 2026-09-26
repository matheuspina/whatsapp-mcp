// Package migrations embeds the versioned SQL migration scripts so the bridge can apply
// them at startup. The .sql files stay the reviewable source of truth; docs/migrations.md
// describes each one.
package migrations

import "embed"

// FS holds every NNN_*.sql migration script.
//
//go:embed *.sql
var FS embed.FS
