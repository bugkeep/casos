package routers

import (
	"bytes"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/beego/beego/context"
	webassets "github.com/casosorg/casos/web"
)

var staticAssets = webassets.Files()

func StaticFilter(ctx *context.Context) {
	urlPath := ctx.Request.URL.Path
	if strings.HasPrefix(urlPath, "/api/") ||
		strings.HasPrefix(urlPath, "/k8s/") ||
		strings.HasPrefix(urlPath, "/.well-known/") ||
		urlPath == "/k8s" {
		return
	}

	assetPath := strings.TrimPrefix(path.Clean(urlPath), "/")
	if assetPath == "." || assetPath == "" {
		assetPath = "index.html"
	}
	if !assetExists(assetPath) && path.Ext(assetPath) == "" {
		assetPath = "index.html"
	}
	if !assetExists(assetPath) {
		return
	}

	content, err := fs.ReadFile(staticAssets, assetPath)
	if err != nil {
		return
	}
	if contentType := mime.TypeByExtension(path.Ext(assetPath)); contentType != "" {
		ctx.ResponseWriter.Header().Set("Content-Type", contentType)
	}
	http.ServeContent(ctx.ResponseWriter, ctx.Request, assetPath, time.Time{}, bytes.NewReader(content))
}

func assetExists(assetPath string) bool {
	info, err := fs.Stat(staticAssets, assetPath)
	return err == nil && !info.IsDir()
}
