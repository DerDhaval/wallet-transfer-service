// Package migrations embeds golang-migrate style *.up.sql / *.down.sql files.
package migrations

import "embed"

// FS holds one-table up/down migration scripts.
//
//go:embed *.sql
var FS embed.FS
