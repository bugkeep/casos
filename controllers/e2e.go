// Copyright 2026 The Casos Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package controllers

import (
	"crypto/subtle"

	"github.com/beego/beego/logs"
	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"

	"github.com/casosorg/casos/conf"
)

const e2eTokenHeader = "X-Casos-E2E-Token"

func (c *ApiController) E2ESignin() {
	e2eEnabled := conf.IsE2ETestMode()
	token := conf.GetConfigString("e2eTestToken")
	if e2eEnabled && token == "" {
		logs.Warning("E2E test mode is enabled but e2eTestToken is not configured")
		c.Ctx.Output.SetStatus(403)
		c.ResponseError("E2E authentication failed")
		return
	}
	if !isE2ESigninAllowed(e2eEnabled, token, c.Ctx.Input.Header(e2eTokenHeader)) {
		logs.Warning("E2E authentication failed")
		c.Ctx.Output.SetStatus(403)
		c.ResponseError("E2E authentication failed")
		return
	}

	owner := conf.GetConfigString("e2eTestOwner")
	if owner == "" {
		logs.Warning("E2E test owner is not configured, defaulting to admin")
		owner = "admin"
	}

	userName := conf.GetConfigString("e2eTestUser")
	if userName == "" {
		userName = "ci-user"
	}
	// This synthetic user is only for CI UI coverage; normal deployments keep this endpoint disabled.
	claims := &casdoorsdk.Claims{
		User: casdoorsdk.User{
			Owner:       owner,
			Name:        userName,
			Id:          owner + "/" + userName,
			Type:        "normal-user",
			DisplayName: "CI User",
			Email:       "ci-user@example.com",
			IsAdmin:     conf.GetConfigBool("e2eTestAdmin"),
		},
	}
	c.SetSessionClaims(claims)
	logs.Info("E2E test sign-in used for user %s", claims.Name)

	c.ResponseOk(claims.User)
}

func isE2ESigninAllowed(enabled bool, expectedToken string, providedToken string) bool {
	if !enabled {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(expectedToken), []byte(providedToken)) == 1
}
