package routers

import (
	"github.com/beego/beego"
	"github.com/casosorg/casos/controllers"
)

func initTestAPI() {
	if !controllers.IsTestSigninEnabled() {
		return
	}

	beego.Router("/api/test-signin", &controllers.ApiController{}, "POST:TestSignin")
}
