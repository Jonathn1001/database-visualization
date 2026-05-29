//go:build !dev

package server

import (
	"embed"
	"io/fs"
)

// distFS holds the built React SPA. Vite writes its output into this package's
// dist/ directory (see web/vite.config.ts outDir). A committed dist/.gitkeep
// ensures this embeds successfully even before a frontend build has run.
//
//go:embed all:dist
var distFS embed.FS

// WebFS returns the embedded SPA filesystem rooted at dist/.
func WebFS() fs.FS {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic(err) // dist/ is embedded at compile time; this cannot fail
	}
	return sub
}
