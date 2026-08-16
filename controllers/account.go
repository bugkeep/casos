package controllers

import (
	"encoding/json"
	"errors"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"

	"github.com/beego/beego"
	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/casosorg/casos/casdoor"
	"github.com/casosorg/casos/conf"
	"github.com/casosorg/casos/object"
)

type localSigninForm struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type localAdminSetupForm struct {
	Password string `json:"password"`
}

type signinOptions struct {
	LocalSigninAvailable  bool              `json:"localSigninAvailable"`
	LocalAdminInitialized bool              `json:"localAdminInitialized"`
	CasdoorConfigured     bool              `json:"casdoorConfigured"`
	AuthConfig            map[string]string `json:"authConfig,omitempty"`
}

const localSessionTokenType = "casos-local"

var casdoorConfigOnce sync.Once

func (c *ApiController) Prepare() {
	claims := c.GetSessionClaims()
	if claims == nil {
		return
	}
	if !sessionMatchesAuthMode(claims, object.IsLocalSigninEnabled(), object.IsCasdoorConfigured(), conf.GetConfigBool("e2eTestMode")) {
		c.SetSessionClaims(nil)
	}
}

func sessionMatchesAuthMode(claims *casdoorsdk.Claims, localSignin, casdoorConfigured, e2eMode bool) bool {
	if claims == nil {
		return false
	}
	e2eIdentity := claims.User.Owner == "built-in" && claims.User.Name == "ci-user"
	if e2eIdentity {
		return e2eMode
	}
	localIdentity := claims.User.Owner == object.LocalUserOwner && claims.User.Type == "local"
	localSession := localIdentity && claims.TokenType == localSessionTokenType
	if localSignin {
		return localSession
	}
	if casdoorConfigured {
		return !localIdentity
	}
	return false
}

func newLocalSessionClaims(user *object.LocalUser) *casdoorsdk.Claims {
	return &casdoorsdk.Claims{
		User:      user.ToSessionUser(),
		TokenType: localSessionTokenType,
	}
}

func buildSigninOptions(localSignin, localAdminInitialized, casdoorConfigured bool) signinOptions {
	options := signinOptions{
		LocalSigninAvailable:  localSignin,
		LocalAdminInitialized: localAdminInitialized,
		CasdoorConfigured:     casdoorConfigured,
	}
	if casdoorConfigured {
		options.AuthConfig = map[string]string{
			"serverUrl":        conf.GetConfigString("casdoorEndpoint"),
			"clientId":         conf.GetConfigString("clientId"),
			"appName":          conf.GetConfigString("casdoorApplication"),
			"organizationName": conf.GetConfigString("casdoorOrganization"),
			"redirectPath":     "/callback",
		}
	}
	return options
}

func (c *ApiController) GetSigninOptions() {
	localAdminInitialized, err := object.IsLocalAdminInitialized()
	if err != nil {
		beego.Error("get local administrator status: %v", err)
		c.ResponseError("failed to load sign-in options")
		return
	}
	c.ResponseOk(buildSigninOptions(object.IsLocalSigninEnabled(), localAdminInitialized, object.IsCasdoorConfigured()))
}

func (c *ApiController) InitializeLocalAdmin() {
	if !object.IsLocalSigninEnabled() {
		c.ResponseError("local sign-in is unavailable")
		return
	}
	if !isLocalSetupRequest(c.Ctx.Request) {
		c.ResponseError("local administrator setup is only available on this device")
		return
	}
	form := localAdminSetupForm{}
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &form); err != nil {
		c.ResponseError("invalid administrator setup request")
		return
	}
	if sanitized, err := json.Marshal(localAdminSetupForm{Password: "***"}); err == nil {
		c.Ctx.Input.RequestBody = sanitized
	}
	if err := object.ValidateLocalPassword(form.Password); err != nil {
		c.ResponseError(err.Error())
		return
	}
	user, err := object.InitializeLocalAdmin(form.Password)
	if errors.Is(err, object.ErrLocalAdminAlreadyInitialized) {
		c.ResponseError(err.Error())
		return
	}
	if err != nil {
		beego.Error("initialize local administrator: %v", err)
		c.ResponseError("failed to initialize local administrator")
		return
	}
	claims := newLocalSessionClaims(user)
	c.SetSessionClaims(claims)
	c.ResponseOk(claims)
}

func (c *ApiController) Signin() {
	code := c.Input().Get("code")
	state := c.Input().Get("state")
	if code == "" && state == "" && object.IsLocalSigninEnabled() {
		c.signinLocalUser()
		return
	}
	if !object.IsCasdoorConfigured() {
		c.ResponseError("Casdoor sign-in is not configured")
		return
	}
	ensureCasdoorConfig()

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

func ensureCasdoorConfig() {
	casdoorConfigOnce.Do(func() {
		casdoorsdk.InitConfig(
			conf.GetConfigString("casdoorEndpoint"),
			conf.GetConfigString("clientId"),
			conf.GetConfigString("clientSecret"),
			casdoor.JwtPublicKey,
			conf.GetConfigString("casdoorOrganization"),
			conf.GetConfigString("casdoorApplication"),
		)
	})
}

func (c *ApiController) signinLocalUser() {
	if !object.IsLocalSigninEnabled() {
		c.ResponseError("local sign-in is unavailable")
		return
	}
	form := localSigninForm{}
	if len(c.Ctx.Input.RequestBody) > 0 {
		if err := json.Unmarshal(c.Ctx.Input.RequestBody, &form); err != nil {
			c.ResponseError("invalid sign-in request")
			return
		}
	}
	form.Username = strings.TrimSpace(form.Username)
	if sanitized, err := json.Marshal(localSigninForm{Username: form.Username, Password: "***"}); err == nil {
		c.Ctx.Input.RequestBody = sanitized
	}
	user, ok, err := object.VerifyLocalUser(form.Username, form.Password)
	if err != nil {
		beego.Error("local sign-in failed: %v", err)
		c.ResponseError("invalid username or password")
		return
	}
	if !ok {
		beego.Warning("local sign-in failed for user %q", form.Username)
		c.ResponseError("invalid username or password")
		return
	}
	claims := newLocalSessionClaims(user)
	c.SetSessionClaims(claims)
	c.ResponseOk(claims)
}

func isLocalSetupRequest(request *http.Request) bool {
	mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || !strings.EqualFold(mediaType, "application/json") {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(request.Header.Get("Sec-Fetch-Site")), "cross-site") {
		return false
	}
	for _, header := range []string{"Forwarded", "X-Forwarded-For", "X-Real-IP"} {
		if strings.TrimSpace(request.Header.Get(header)) != "" {
			return false
		}
	}
	if !isLoopbackAddress(request.RemoteAddr) {
		return false
	}
	host := request.Host
	if parsedHost, _, err := net.SplitHostPort(host); err == nil {
		host = parsedHost
	}
	if !strings.EqualFold(host, "localhost") && !isLoopbackAddress(host) {
		return false
	}
	origin := strings.TrimSpace(request.Header.Get("Origin"))
	if origin == "" {
		return true
	}
	parsedOrigin, err := url.Parse(origin)
	expectedScheme := "http"
	if request.TLS != nil {
		expectedScheme = "https"
	}
	if err != nil || parsedOrigin.Scheme != expectedScheme || parsedOrigin.User != nil || parsedOrigin.Path != "" || parsedOrigin.RawQuery != "" || parsedOrigin.Fragment != "" {
		return false
	}
	return strings.EqualFold(parsedOrigin.Host, request.Host)
}

func isLoopbackAddress(address string) bool {
	parsedHost, _, err := net.SplitHostPort(address)
	if err == nil {
		address = parsedHost
	}
	ip := net.ParseIP(address)
	return ip != nil && ip.IsLoopback()
}

func (c *ApiController) Signout() {
	c.SetSessionClaims(nil)

	c.ResponseOk()
}

func (c *ApiController) GetAccount() {
	if c.RequireSignedIn() {
		return
	}

	claims := c.GetSessionClaims()
	hostname, _ := os.Hostname()

	c.ResponseOk(claims, hostname)
}
