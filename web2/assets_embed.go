//go:build embed

package web2assets

import "io/fs"

// A standalone binary still ships the Ant Design frontend from web/build: the
// shadcn rewrite is not yet feature-complete, and embedding it would require
// every `-tags embed` build to run `yarn build` in web2 as well.
//
// To ship web2 instead, replace the body of this file with the embed directive
// from web/assets_embed.go pointed at web2/build, and add the web2 build to CI
// next to the existing web one.
func Available() bool {
	return false
}

func Files() fs.FS {
	return nil
}
