package routers

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/beego/beego"
	"github.com/beego/beego/context"
)

func firstForwardedValue(value string) string {
	return strings.TrimSpace(strings.Split(value, ",")[0])
}

func requestOrigin(ctx *context.Context) string {
	if publicOrigin := strings.TrimSpace(beego.AppConfig.String("publicOrigin")); publicOrigin != "" {
		return publicOrigin
	}
	scheme := "http"
	if ctx.Request.TLS != nil {
		scheme = "https"
	}
	host := ctx.Request.Host
	return scheme + "://" + host
}

func isAllowedCORSOrigin(ctx *context.Context, origin string) bool {
	originURL, err := url.Parse(origin)
	if err != nil || originURL.Scheme == "" || originURL.Host == "" {
		return false
	}
	requestURL, err := url.Parse(requestOrigin(ctx))
	if err == nil && strings.EqualFold(originURL.Scheme, requestURL.Scheme) && strings.EqualFold(originURL.Host, requestURL.Host) {
		return true
	}
	return origin == "http://localhost:8001" || origin == "http://127.0.0.1:8001"
}

func CorsFilter(ctx *context.Context) {
	origin := strings.TrimSpace(ctx.Request.Header.Get("Origin"))
	if origin != "" {
		if !isAllowedCORSOrigin(ctx, origin) {
			ctx.ResponseWriter.WriteHeader(http.StatusForbidden)
			return
		}
		ctx.ResponseWriter.Header().Set("Access-Control-Allow-Origin", origin)
		ctx.ResponseWriter.Header().Set("Access-Control-Allow-Credentials", "true")
	}
	ctx.ResponseWriter.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, PUT, PATCH, OPTIONS")
	ctx.ResponseWriter.Header().Set("Access-Control-Allow-Headers", "Origin, X-Requested-With, Content-Type, Accept")
	if ctx.Request.Method == http.MethodOptions {
		ctx.ResponseWriter.WriteHeader(http.StatusOK)
	}
}
