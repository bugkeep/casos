package controllers

import (
	"github.com/beego/beego/logs"

	"github.com/casosorg/casos/auth"
	"github.com/casosorg/casos/conf"
)

const e2eTokenHeader = "X-Casos-E2E-Token"

func (c *ApiController) E2ESignin() {
	if !conf.GetConfigBool("e2eTestMode") {
		c.ResponseError("E2E test mode is disabled")
		return
	}

	token := conf.GetConfigString("e2eTestToken")
	if token == "" {
		c.ResponseError("E2E test token is not configured")
		return
	}
	if c.Ctx.Input.Header(e2eTokenHeader) != token {
		c.ResponseError("invalid E2E token")
		return
	}

	identity := &auth.SessionIdentity{
		User: auth.User{
			Owner:       "built-in",
			Name:        "ci-user",
			DisplayName: "CI User",
			IsAdmin:     true,
			Provider:    "e2e",
		},
	}
	if err := c.establishSession(identity); err != nil {
		c.ResponseError("could not establish session")
		return
	}
	logs.Info("E2E test sign-in used for user %s", identity.User.Name)

	c.ResponseOk(identity.User)
}
