package routers

import (
	"github.com/beego/beego/v2/server/web"
	"github.com/casosorg/casos/controllers"
)

func InitAPI() {
	web.Router("/api/get-pods", &controllers.ApiController{}, "GET:GetPods")
}
