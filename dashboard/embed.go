// Package dashboard embarque le build du dashboard (React, dans dist/), servi par cfa-api sur GET /.
//
// dist/ est produit par « npm run build » (make ui, ou l'image Docker de cfa-api). Sans build,
// seul dist/.gitkeep existe : le package compile et cfa-api démarre sans dashboard.
package dashboard

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// FS renvoie les fichiers du dashboard, ou nil s'il n'a pas été construit.
func FS() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		return nil
	}
	if _, err := fs.Stat(sub, "index.html"); err != nil {
		return nil
	}
	return sub
}
