//go:build embed

package webassets

import (
	"embed"
	"io/fs"
)

// The all: prefix keeps entries whose names begin with "." or "_", which a
// plain pattern silently drops: everything under web/public is copied into the
// build output verbatim, so the frontend decides what ships, not this pattern.
//
//go:embed all:build
var embedded embed.FS

// Files returns the frontend compiled into the binary. Building with
// -tags embed therefore requires web/build to exist: run `yarn build` in web
// first.
func Files() fs.FS {
	files, err := fs.Sub(embedded, "build")
	if err != nil {
		// "build" is a constant that fs.Sub accepts by construction; an error
		// here would mean the embedded tree itself is malformed.
		panic(err)
	}
	return files
}
