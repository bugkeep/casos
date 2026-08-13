package routers

import (
	"net/http"

	"github.com/beego/beego/context"
	"github.com/casosorg/casos/security"
)

func CorsFilter(ctx *context.Context) {
	origin := ctx.Request.Header.Get("Origin")
	if !security.IsAllowedOrigin(origin, ctx.Request) {
		responseErrorStatus(ctx, http.StatusForbidden, "origin is not allowed")
		return
	}
	if origin != "" {
		ctx.ResponseWriter.Header().Set("Access-Control-Allow-Origin", origin)
		ctx.ResponseWriter.Header().Set("Vary", "Origin")
	}
	ctx.ResponseWriter.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, PUT, PATCH, OPTIONS")
	ctx.ResponseWriter.Header().Set("Access-Control-Allow-Headers", "Origin, X-Requested-With, Content-Type, Accept, X-CSRF-Token")
	ctx.ResponseWriter.Header().Set("Access-Control-Allow-Credentials", "true")
	if ctx.Request.Method == http.MethodOptions {
		ctx.ResponseWriter.WriteHeader(http.StatusNoContent)
	}
}
