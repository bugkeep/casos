package routers

import (
	"net/http"
	"path/filepath"
	"strings"

	"github.com/beego/beego/context"
	"github.com/beego/beego/logs"
	"github.com/casosorg/casos/util"
)

func StaticFilter(ctx *context.Context) {
	urlPath := ctx.Request.URL.Path
	if strings.HasPrefix(urlPath, "/api/") ||
		strings.HasPrefix(urlPath, "/k8s/") ||
		strings.HasPrefix(urlPath, "/.well-known/") ||
		urlPath == "/k8s" {
		return
	}

	path := "web/build"
	if urlPath == "/" {
		path += "/index.html"
	} else {
		path += urlPath
	}

	if fileExists(path) {
		if urlPath == "/" {
			if err := util.AppendWebConfigCookie(ctx); err != nil {
				logs.Error("AppendWebConfigCookie() error: %s", err.Error())
			}
		}
		http.ServeFile(ctx.ResponseWriter, ctx.Request, path)
	} else if filepath.Ext(urlPath) == "" {
		// Unknown path without extension — let React router handle it.
		if err := util.AppendWebConfigCookie(ctx); err != nil {
			logs.Error("AppendWebConfigCookie() error: %s", err.Error())
		}
		http.ServeFile(ctx.ResponseWriter, ctx.Request, "web/build/index.html")
	}
}

func fileExists(path string) bool {
	f, err := http.Dir(".").Open(path)
	if err != nil {
		return false
	}
	f.Close()
	return true
}
