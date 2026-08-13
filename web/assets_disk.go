//go:build !embed

package webassets

import (
	"io/fs"
	"os"
)

func Files() fs.FS {
	return os.DirFS("web/build")
}
