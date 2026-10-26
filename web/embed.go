// Package web embeds the built SvelteKit SPA (web/build) into lor-server.
// Build it first with `make web`; go build fails loudly if build/ is missing.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:build
var build embed.FS

// Files returns the built SPA rooted at build/.
func Files() fs.FS {
	sub, err := fs.Sub(build, "build")
	if err != nil {
		panic(err)
	}
	return sub
}
