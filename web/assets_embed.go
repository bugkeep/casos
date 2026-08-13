//go:build embed

package webassets

import (
	"embed"
	"io/fs"
)

//go:embed build
var embedded embed.FS

func Files() fs.FS {
	files, err := fs.Sub(embedded, "build")
	if err != nil {
		panic(err)
	}
	return files
}
