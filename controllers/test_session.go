package controllers

import (
	"os"
	"strings"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
)

func IsTestSigninEnabled() bool {
	return strings.TrimSpace(os.Getenv("CASOS_TEST_LOGIN_ENABLED")) != ""
}

func newTestSigninClaims() *casdoorsdk.Claims {
	return &casdoorsdk.Claims{
		User: casdoorsdk.User{
			Owner:       "casos",
			Name:        "ci-admin",
			DisplayName: "CI Admin",
			Email:       "ci@example.com",
			IsAdmin:     true,
		},
		AccessToken: "casos-test-login",
	}
}

func (c *ApiController) TestSignin() {
	if !IsTestSigninEnabled() {
		c.ResponseError("test signin is disabled")
		return
	}

	claims := newTestSigninClaims()
	c.SetSessionClaims(claims)
	c.ResponseOk(claims)
}
