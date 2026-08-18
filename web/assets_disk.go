//go:build !embed

// Package webassets exposes the frontend in web/build.
package webassets

import (
	"io/fs"
	"os"
	"path/filepath"
)

const diskAssetDir = "web/build"

// Files reads the frontend from web/build on disk, so a `yarn build` is picked
// up without recompiling the backend.
func Files() fs.FS {
	return os.DirFS(assetDir())
}

// assetDir locates web/build the way beego locates conf/app.conf: relative to
// the working directory first, then next to the executable. Without the second
// rule a backend binary copied out of the repository serves a blank page
// instead of the frontend shipped beside it.
func assetDir() string {
	if isDir(diskAssetDir) {
		return diskAssetDir
	}
	if executable, err := os.Executable(); err == nil {
		if dir := filepath.Join(filepath.Dir(executable), diskAssetDir); isDir(dir) {
			return dir
		}
	}
	return diskAssetDir
}

func isDir(name string) bool {
	info, err := os.Stat(name)
	return err == nil && info.IsDir()
}
