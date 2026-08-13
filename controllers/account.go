package controllers

import (
	"errors"
	"net/http"
	"os"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/casosorg/casos/auth"
	"github.com/casosorg/casos/conf"
)

func (c *ApiController) Signin() {
	provider, err := conf.GetAuthProvider()
	if err != nil || provider != "casdoor" {
		c.ResponseErrorStatus(http.StatusNotFound, "Casdoor authentication is not enabled")
		return
	}
	code := c.Input().Get("code")
	state := c.Input().Get("state")

	token, err := casdoorsdk.GetOAuthToken(code, state)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}

	claims, err := casdoorsdk.ParseJwtToken(token.AccessToken)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}

	identity := auth.FromCasdoorUser(&claims.User)
	if err = c.establishSession(identity); err != nil {
		c.ResponseErrorStatus(http.StatusInternalServerError, "could not establish session")
		return
	}

	c.ResponseOk(identity.User)
}

func (c *ApiController) Signout() {
	c.DestroySession()
	c.ResponseOk()
}

func (c *ApiController) GetAccount() {
	if c.RequireSignedIn() {
		return
	}

	user := c.GetSessionUser()
	hostname, _ := os.Hostname()

	c.ResponseOk(user, hostname)
}

func (c *ApiController) establishSession(identity *auth.SessionIdentity) error {
	if identity == nil {
		return errors.New("session identity is nil")
	}
	token, err := auth.NewCSRFToken()
	if err != nil {
		return err
	}
	identity.CSRFToken = token
	if err = c.SessionRegenerateID(); err != nil {
		return err
	}
	c.SetSessionIdentity(identity)
	return nil
}
