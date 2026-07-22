package routers

import (
	"github.com/beego/beego/context"
	"github.com/casosorg/casos/conf"
)

func ApiFilter(ctx *context.Context) {
	if conf.IsDemoMode() && !isAllowedInDemoMode(ctx.Request.Method, ctx.Request.URL.Path) {
		denyRequest(ctx)
	}
}

func isAllowedInDemoMode(method, urlPath string) bool {
	if method == "POST" {
		return urlPath == "/api/signin" || urlPath == "/api/signout"
	}
	return true
}
