package controllers

import (
	"encoding/json"

	"github.com/casosorg/casos/conf"
	"github.com/casosorg/casos/object"
)

type signinForm struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type accountForm struct {
	DisplayName     string `json:"displayName"`
	Avatar          string `json:"avatar"`
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

type signinOptions struct {
	CasdoorAvailable bool              `json:"casdoorAvailable"`
	SigninAvailable  bool              `json:"signinAvailable"`
	AutoSignin       bool              `json:"autoSignin"`
	AuthConfig       map[string]string `json:"authConfig,omitempty"`
}

// GetSigninOptions tells the web UI which sign-in method the server is running:
// Casdoor OAuth, the built-in password form, or neither.
func (c *ApiController) GetSigninOptions() {
	signinEnabled := object.IsSigninEnabled()
	casdoorAvailable := conf.IsCasdoorAvailable()

	options := signinOptions{
		CasdoorAvailable: casdoorAvailable,
		SigninAvailable:  signinEnabled,
		AutoSignin:       signinEnabled && object.IsAdminUsingDefaultPassword(),
	}
	if casdoorAvailable {
		// The client secret is deliberately not part of this payload.
		options.AuthConfig = map[string]string{
			"serverUrl":        conf.GetConfigString("casdoorEndpoint"),
			"clientId":         conf.GetConfigString("clientId"),
			"appName":          conf.GetConfigString("casdoorApplication"),
			"organizationName": conf.GetConfigString("casdoorOrganization"),
			"redirectPath":     "/callback",
		}
	}

	c.ResponseOk(options)
}

// UpdateAccount updates the signed-in built-in user's profile, and its password
// when a new one is supplied.
func (c *ApiController) UpdateAccount() {
	sessionUser := c.GetSessionUser()
	if sessionUser == nil || sessionUser.Owner != object.UserOwner {
		c.ResponseError("unauthorized operation")
		return
	}

	form := accountForm{}
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &form); err != nil {
		c.ResponseError(err.Error())
		return
	}
	if form.CurrentPassword != "" || form.NewPassword != "" {
		if sanitizedBody, err := json.Marshal(accountForm{DisplayName: form.DisplayName, Avatar: form.Avatar, CurrentPassword: "***", NewPassword: "***"}); err == nil {
			c.Ctx.Input.RequestBody = sanitizedBody
		}
	}

	accountUser, err := object.GetUserByRuntimeName(sessionUser.Name)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	if accountUser == nil {
		c.ResponseError("unauthorized operation")
		return
	}

	if form.NewPassword != "" && !object.CheckUserPassword(accountUser, form.CurrentPassword) {
		c.ResponseError("invalid username or password")
		return
	}

	accountUser.DisplayName = form.DisplayName
	accountUser.Avatar = form.Avatar
	if err = object.UpdateUserProfile(accountUser); err != nil {
		c.ResponseError(err.Error())
		return
	}
	if form.NewPassword != "" {
		if err = object.UpdateUserPassword(accountUser, form.NewPassword); err != nil {
			c.ResponseError(err.Error())
			return
		}
	}

	user := accountUser.ToCasdoorUser()
	c.SetSessionClaims(newSigninClaims(user))
	c.ResponseOk(user)
}

func (c *ApiController) signinWithPassword() {
	if !object.IsSigninEnabled() {
		c.ResponseError("sign in is unavailable")
		return
	}

	form := signinForm{}
	if len(c.Ctx.Input.RequestBody) > 0 {
		if err := json.Unmarshal(c.Ctx.Input.RequestBody, &form); err != nil {
			c.ResponseError(err.Error())
			return
		}
	}
	if form.Username == "" {
		form.Username = c.Input().Get("username")
	}
	if form.Password == "" {
		form.Password = c.Input().Get("password")
	}
	if sanitizedBody, err := json.Marshal(signinForm{Username: form.Username, Password: "***"}); err == nil {
		c.Ctx.Input.RequestBody = sanitizedBody
	}

	accountUser, ok, err := object.VerifyUser(form.Username, form.Password)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	if !ok {
		c.ResponseError("invalid username or password")
		return
	}

	claims := newSigninClaims(accountUser.ToCasdoorUser())
	c.SetSessionClaims(claims)
	c.ResponseOk(claims)
}
