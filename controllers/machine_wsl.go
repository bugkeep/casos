package controllers

import (
	"github.com/casosorg/casos/deploy"
)

// AddLocalWSLMachine enrolls the WSL distro of the local Windows host as a
// machine, without asking the user for any connection detail.
// @router /api/add-local-wsl-machine [post]
func (c *ApiController) AddLocalWSLMachine() {
	if c.RequireAdmin() {
		return
	}

	result, err := deploy.AddLocalWSLMachine(c.Ctx.Request.Context(), "admin", "")
	if err != nil {
		c.ResponseError(err.Error())
		return
	}

	c.ResponseOk(result)
}
