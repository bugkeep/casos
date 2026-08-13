package controllers

import (
	"github.com/beego/beego"
	"github.com/casosorg/casos/auth"
	"github.com/casosorg/casos/object"
)

type ApiController struct {
	beego.Controller
}

func (c *ApiController) GetSessionIdentity() *auth.SessionIdentity {
	identity, legacy := auth.NormalizeSession(c.GetSession("user"))
	if identity == nil {
		return nil
	}
	if identity.User.Provider == "local" && !object.IsLocalSessionCurrent(identity.SessionVersion) {
		c.DelSession("user")
		return nil
	}
	if legacy {
		c.SetSession("user", *identity)
	}
	return identity
}

func (c *ApiController) SetSessionIdentity(identity *auth.SessionIdentity) {
	if identity == nil {
		c.DelSession("user")
		return
	}
	c.SetSession("user", *identity)
}

func (c *ApiController) GetSessionUser() *auth.User {
	identity := c.GetSessionIdentity()
	if identity == nil {
		return nil
	}
	return &identity.User
}
