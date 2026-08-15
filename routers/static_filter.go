package routers

import (
	"bytes"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"

	"github.com/beego/beego/context"
	webassets "github.com/casosorg/casos/web"
)

const indexFile = "index.html"

// staticAssets is the compiled frontend: embedded in the binary for standalone
// builds, read from web/build on disk for every other build.
var staticAssets = webassets.Files()

func init() {
	// Windows resolves MIME types through the registry, where .js is routinely
	// registered as text/plain — which browsers refuse to execute as a script.
	// Pin the types the frontend depends on so the UI loads on every platform.
	for extension, contentType := range map[string]string{
		".js":   "text/javascript; charset=utf-8",
		".css":  "text/css; charset=utf-8",
		".json": "application/json",
		".svg":  "image/svg+xml",
	} {
		_ = mime.AddExtensionType(extension, contentType)
	}
}

func StaticFilter(ctx *context.Context) {
	urlPath := ctx.Request.URL.Path
	if strings.HasPrefix(urlPath, "/api/") ||
		strings.HasPrefix(urlPath, "/k8s/") ||
		strings.HasPrefix(urlPath, "/.well-known/") ||
		urlPath == "/k8s" {
		return
	}

	name := strings.TrimPrefix(path.Clean(urlPath), "/")
	if name == "" || name == "." {
		name = indexFile
	}

	file, err := openAsset(name)
	if err != nil {
		// An unknown path without an extension is a React router route that
		// index.html resolves client side. A missing path with an extension is
		// a genuine 404 and is left to beego.
		if path.Ext(name) != "" {
			return
		}
		name = indexFile
		if file, err = openAsset(name); err != nil {
			return
		}
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil || info.IsDir() {
		return
	}
	setCacheControl(ctx.ResponseWriter, name)

	// ServeContent picks the content type from the name, and streams while
	// honouring range and conditional requests whenever the asset can seek —
	// which both the embedded and the on-disk file systems can.
	if seeker, ok := file.(io.ReadSeeker); ok {
		http.ServeContent(ctx.ResponseWriter, ctx.Request, name, info.ModTime(), seeker)
		return
	}
	content, err := io.ReadAll(file)
	if err != nil {
		return
	}
	http.ServeContent(ctx.ResponseWriter, ctx.Request, name, info.ModTime(), bytes.NewReader(content))
}

func openAsset(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, fs.ErrInvalid
	}
	return staticAssets.Open(name)
}

// setCacheControl applies the policy the frontend build implies: files under
// static/ carry a content hash in their name and never change, while
// index.html and the remaining top-level assets keep their names across
// releases and must be revalidated or an upgrade serves a stale app.
func setCacheControl(writer http.ResponseWriter, name string) {
	if strings.HasPrefix(name, "static/") {
		writer.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		return
	}
	writer.Header().Set("Cache-Control", "no-cache")
}
