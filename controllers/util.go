package controllers

import "net/http"

type Response struct {
	Status string      `json:"status"`
	Msg    string      `json:"msg"`
	Data   interface{} `json:"data"`
	Data2  interface{} `json:"data2"`
}

func (c *ApiController) ResponseOk(data ...interface{}) {
	resp := Response{Status: "ok"}
	switch len(data) {
	case 2:
		resp.Data2 = data[1]
		fallthrough
	case 1:
		resp.Data = data[0]
	}
	c.Data["json"] = resp
	c.ServeJSON()
}

func (c *ApiController) ResponseError(error string, data ...interface{}) {
	c.ResponseErrorStatus(http.StatusBadRequest, error, data...)
}

func (c *ApiController) ResponseErrorStatus(status int, message string, data ...interface{}) {
	resp := Response{Status: "error", Msg: message}
	switch len(data) {
	case 2:
		resp.Data2 = data[1]
		fallthrough
	case 1:
		resp.Data = data[0]
	}
	c.Data["json"] = resp
	c.Ctx.Output.SetStatus(status)
	c.ServeJSON()
}

func (c *ApiController) RequireSignedIn() bool {
	if c.GetSessionUser() == nil {
		c.ResponseErrorStatus(http.StatusUnauthorized, "please sign in first")
		return true
	}

	return false
}

func (c *ApiController) RequireAdmin() bool {
	return c.RequireSignedIn()
}
