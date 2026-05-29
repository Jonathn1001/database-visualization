//go:build dev

package server

import "io/fs"

// WebFS returns nil in dev builds: the SPA is served by the Vite dev server on
// port 5173, which proxies /api to this Go server. The router skips the static
// file handler when Config.Dev is true.
func WebFS() fs.FS {
	return nil
}
