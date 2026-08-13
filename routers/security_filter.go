package routers

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/beego/beego/context"
	"github.com/casosorg/casos/auth"
	"github.com/casosorg/casos/object"
)

var publicAPIPaths = map[string]bool{
	"/api/auth/status":       true,
	"/api/auth/setup":        true,
	"/api/auth/login":        true,
	"/api/auth/recover":      true,
	"/api/signin":            true,
	"/api/e2e/signin":        true,
	"/api/get-built-in-site": true,
}

func SecurityFilter(ctx *context.Context) {
	path := ctx.Request.URL.Path
	if !strings.HasPrefix(path, "/api/") && path != "/k8s" && !strings.HasPrefix(path, "/k8s/") {
		return
	}

	identity, legacy := auth.NormalizeSession(ctx.Input.CruSession.Get("user"))
	if identity != nil && identity.User.Provider == "local" && !object.IsLocalSessionCurrent(identity.SessionVersion) {
		_ = ctx.Input.CruSession.Delete("user")
		identity = nil
	}
	if identity != nil && legacy {
		_ = ctx.Input.CruSession.Set("user", *identity)
	}

	publicPath := publicAPIPaths[path]
	if ctx.Request.Method == http.MethodOptions || (publicPath && (identity == nil || !isUnsafeMethod(ctx.Request.Method))) ||
		path == "/api/signin" || path == "/api/e2e/signin" {
		return
	}
	if identity != nil && isUnsafeMethod(ctx.Request.Method) {
		supplied := ctx.Request.Header.Get("X-CSRF-Token")
		if identity.CSRFToken == "" || subtle.ConstantTimeCompare([]byte(supplied), []byte(identity.CSRFToken)) != 1 {
			responseErrorStatus(ctx, http.StatusForbidden, "invalid CSRF token")
			return
		}
	}
	if publicPath {
		return
	}
	if identity == nil {
		responseErrorStatus(ctx, http.StatusUnauthorized, "please sign in first")
	}
}

func isUnsafeMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}
