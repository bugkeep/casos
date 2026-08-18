//go:build !embed

// Package web2assets exposes the shadcn frontend in web2/build the same way
// package webassets exposes the Ant Design one in web/build.
package web2assets

import (
	"io/fs"
	"os"
	"path/filepath"
)

const diskAssetDir = "web2/build"

// Available reports whether web2 has been built. The router uses it to decide
// which frontend to serve, so a checkout that has never run `yarn build` in
// web2 keeps serving the old UI instead of a directory listing of nothing.
func Available() bool {
	return assetDir() != ""
}

// Files reads the frontend from web2/build on disk, so a `yarn build` is picked
// up without recompiling the backend. It returns nil when web2 has not been
// built; callers must check Available first.
func Files() fs.FS {
	dir := assetDir()
	if dir == "" {
		return nil
	}
	return os.DirFS(dir)
}

// assetDir locates web2/build the way webassets locates web/build: relative to
// the working directory first, then next to the executable, so a backend binary
// copied out of the repository still finds the frontend shipped beside it.
func assetDir() string {
	if isDir(diskAssetDir) {
		return diskAssetDir
	}
	if executable, err := os.Executable(); err == nil {
		if dir := filepath.Join(filepath.Dir(executable), diskAssetDir); isDir(dir) {
			return dir
		}
	}
	return ""
}

func isDir(name string) bool {
	info, err := os.Stat(name)
	return err == nil && info.IsDir()
}
