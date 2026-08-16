package controllers

import (
	"os"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/casosorg/casos/conf"
	"github.com/casosorg/casos/object"
)

func newSigninClaims(user casdoorsdk.User) *casdoorsdk.Claims {
	return &casdoorsdk.Claims{User: user}
}

func (c *ApiController) Signin() {
	code := c.Input().Get("code")
	state := c.Input().Get("state")
	if code == "" && state == "" {
		c.signinWithPassword()
		return
	}

	if !conf.IsCasdoorAvailable() {
		c.ResponseError("Casdoor sign-in is not configured")
		return
	}

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

	claims.AccessToken = token.AccessToken
	c.SetSessionClaims(claims)

	c.ResponseOk(claims)
}

func (c *ApiController) Signout() {
	c.SetSessionClaims(nil)

	c.ResponseOk()
}

// autoLoginAdmin signs the built-in admin in without asking for credentials, and
// is only ever reached while that account still carries its default password.
// Returns false when the response has already been written.
func (c *ApiController) autoLoginAdmin() bool {
	accountUser, ok, err := object.VerifyUser(object.DefaultAdminName, object.DefaultAdminPassword)
	if err != nil {
		c.ResponseError(err.Error())
		return false
	}
	if !ok {
		c.ResponseError("please sign in first")
		return false
	}

	c.SetSessionClaims(newSigninClaims(accountUser.ToCasdoorUser()))
	return true
}

func (c *ApiController) GetAccount() {
	if object.IsSigninEnabled() {
		if c.GetSessionUser() == nil {
			// The sign-in page asks for the account on purpose, so never sign the
			// visitor in from underneath the form they are looking at.
			fromPath := c.GetString("fromPath")
			if fromPath != "/signin" && object.IsAdminUsingDefaultPassword() {
				if !c.autoLoginAdmin() {
					return
				}
			} else {
				c.ResponseError("please sign in first")
				return
			}
		}
	} else if c.RequireSignedIn() {
		return
	}

	claims := c.GetSessionClaims()
	hostname, _ := os.Hostname()

	c.ResponseOk(claims, hostname)
}
