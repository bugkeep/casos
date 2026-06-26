package controllers

import (
	"github.com/beego/beego/logs"
	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"

	"github.com/casosorg/casos/conf"
)

const e2eTokenHeader = "X-Casos-E2E-Token"

func (c *ApiController) E2ESignin() {
	token := conf.GetConfigString("e2eTestToken")
	if !conf.GetConfigBool("e2eTestMode") || token == "" || c.Ctx.Input.Header(e2eTokenHeader) != token {
		c.ResponseError("E2E authentication failed")
		return
	}

	owner := conf.GetConfigString("e2eTestOwner")
	if owner == "" {
		owner = "built-in"
	}

	claims := &casdoorsdk.Claims{
		User: casdoorsdk.User{
			Owner:       owner,
			Name:        "ci-user",
			DisplayName: "CI User",
			IsAdmin:     conf.GetConfigBool("e2eTestAdmin"),
		},
	}
	c.SetSessionClaims(claims)
	logs.Info("E2E test sign-in used for user %s", claims.Name)

	c.ResponseOk(claims.User)
}
