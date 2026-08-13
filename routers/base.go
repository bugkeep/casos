package routers

import (
	"net/http"

	"github.com/beego/beego/context"
	"github.com/casosorg/casos/controllers"
)

func responseErrorStatus(ctx *context.Context, status int, message string, data ...interface{}) {
	resp := controllers.Response{Status: "error", Msg: message}
	switch len(data) {
	case 2:
		resp.Data2 = data[1]
		fallthrough
	case 1:
		resp.Data = data[0]
	}

	ctx.Output.SetStatus(status)
	err := ctx.Output.JSON(resp, true, false)
	if err != nil {
		panic(err)
	}
}

func denyRequest(ctx *context.Context) {
	responseErrorStatus(ctx, http.StatusForbidden, "Unauthorized operation")
}
