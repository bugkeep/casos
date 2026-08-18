//go:build embed

package web2assets

import (
	"embed"
	"io/fs"
)

// The all: prefix keeps entries whose names begin with "." or "_", which a
// plain pattern silently drops: everything under web2/public is copied into the
// build output verbatim, so the frontend decides what ships, not this pattern.
//
//go:embed all:build
var embedded embed.FS

// Available reports that a standalone binary carries this frontend. It is a
// constant under this build tag: the embed directive above will not compile
// unless web2/build exists, so if the binary linked, the assets are in it.
func Available() bool {
	return true
}

// Files returns the frontend compiled into the binary. Building with
// -tags embed therefore requires web2/build to exist: run `yarn build` in web2
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
