// Package migrations embarque le schéma SQL versionné (goose).
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
