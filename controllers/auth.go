package controllers

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/casosorg/casos/auth"
	"github.com/casosorg/casos/conf"
	"github.com/casosorg/casos/object"
)

const (
	loginAttemptWindow = 15 * time.Minute
	maxLoginFailures   = 5
)

type authRequest struct {
	Username        string `json:"username"`
	Password        string `json:"password"`
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

type loginFailure struct {
	Count       int
	WindowStart time.Time
	LockedUntil time.Time
}

type loginLimiter struct {
	sync.Mutex
	failures map[string]loginFailure
}

var localLoginLimiter = loginLimiter{failures: map[string]loginFailure{}}

func (c *ApiController) AuthStatus() {
	provider, err := conf.GetAuthProvider()
	if err != nil {
		c.ResponseErrorStatus(http.StatusInternalServerError, err.Error())
		return
	}
	identity := c.GetSessionIdentity()
	initialized := provider == "casdoor" || (identity != nil && identity.User.Provider == "e2e")
	if provider == "local" {
		var localInitialized bool
		localInitialized, err = object.LocalAdminExists()
		if err != nil {
			c.ResponseErrorStatus(http.StatusInternalServerError, "could not read authentication state")
			return
		}
		initialized = initialized || localInitialized
	}

	if identity != nil && identity.CSRFToken == "" {
		identity.CSRFToken, err = auth.NewCSRFToken()
		if err != nil {
			c.ResponseErrorStatus(http.StatusInternalServerError, "could not create CSRF token")
			return
		}
		c.SetSessionIdentity(identity)
	}

	data := map[string]interface{}{
		"provider":      provider,
		"initialized":   initialized,
		"authenticated": identity != nil,
		"canRecover":    provider == "local" && isTrueLocalRequest(c.Ctx.Request),
		"canSetup":      provider == "local" && !initialized && isTrueLocalRequest(c.Ctx.Request),
	}
	if identity != nil {
		data["user"] = identity.User
		data["csrfToken"] = identity.CSRFToken
	}
	if provider == "casdoor" {
		data["casdoor"] = map[string]string{
			"serverUrl":        conf.GetConfigString("casdoorEndpoint"),
			"clientId":         conf.GetConfigString("clientId"),
			"organizationName": conf.GetConfigString("casdoorOrganization"),
			"appName":          conf.GetConfigString("casdoorApplication"),
			"redirectPath":     "/callback",
		}
	}
	c.ResponseOk(data)
}

func (c *ApiController) AuthSetup() {
	if !c.requireLocalProvider() {
		return
	}
	if !isTrueLocalRequest(c.Ctx.Request) {
		c.ResponseErrorStatus(http.StatusForbidden, "initial setup is only available from the local computer while CasOS listens on loopback")
		return
	}
	var request authRequest
	if !c.decodeAuthRequest(&request) {
		return
	}
	admin, err := object.SetupLocalAdmin(request.Username, request.Password)
	if errors.Is(err, object.ErrLocalAdminExists) {
		c.ResponseErrorStatus(http.StatusConflict, "local administrator is already initialized")
		return
	}
	if err != nil {
		c.ResponseErrorStatus(http.StatusBadRequest, err.Error())
		return
	}
	if err = c.establishSession(localAdminIdentity(admin)); err != nil {
		c.ResponseErrorStatus(http.StatusInternalServerError, "could not establish session")
		return
	}
	c.ResponseOk(c.GetSessionUser())
}

func (c *ApiController) AuthLogin() {
	if !c.requireLocalProvider() {
		return
	}
	var request authRequest
	if !c.decodeAuthRequest(&request) {
		return
	}
	key := requestSourceIP(c.Ctx.Request) + "|" + strings.ToLower(strings.TrimSpace(request.Username))
	if localLoginLimiter.isLocked(key, time.Now()) {
		c.ResponseErrorStatus(http.StatusTooManyRequests, "too many failed sign-in attempts; try again later")
		return
	}
	admin, err := object.AuthenticateLocalAdmin(request.Username, request.Password)
	if err != nil {
		locked := localLoginLimiter.fail(key, time.Now())
		if locked {
			c.ResponseErrorStatus(http.StatusTooManyRequests, "too many failed sign-in attempts; try again later")
		} else {
			c.ResponseErrorStatus(http.StatusUnauthorized, "invalid username or password")
		}
		return
	}
	localLoginLimiter.clear(key)
	if err = c.establishSession(localAdminIdentity(admin)); err != nil {
		c.ResponseErrorStatus(http.StatusInternalServerError, "could not establish session")
		return
	}
	c.ResponseOk(c.GetSessionUser())
}

func (c *ApiController) AuthLogout() {
	c.Signout()
}

func (c *ApiController) AuthPassword() {
	if !c.requireLocalProvider() {
		return
	}
	if c.RequireSignedIn() {
		return
	}
	var request authRequest
	if !c.decodeAuthRequest(&request) {
		return
	}
	admin, err := object.ChangeLocalAdminPassword(request.CurrentPassword, request.NewPassword)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, object.ErrInvalidCredentials) {
			status = http.StatusUnauthorized
		}
		c.ResponseErrorStatus(status, err.Error())
		return
	}
	if err = c.establishSession(localAdminIdentity(admin)); err != nil {
		c.ResponseErrorStatus(http.StatusInternalServerError, "could not establish session")
		return
	}
	c.ResponseOk(c.GetSessionUser())
}

func (c *ApiController) AuthRecover() {
	if !c.requireLocalProvider() {
		return
	}
	if !isTrueLocalRequest(c.Ctx.Request) {
		c.ResponseErrorStatus(http.StatusForbidden, "password recovery is only available from the local computer while CasOS listens on loopback")
		return
	}
	var request authRequest
	if !c.decodeAuthRequest(&request) {
		return
	}
	admin, err := object.RecoverLocalAdminPassword(request.NewPassword)
	if err != nil {
		c.ResponseErrorStatus(http.StatusBadRequest, err.Error())
		return
	}
	if err = c.establishSession(localAdminIdentity(admin)); err != nil {
		c.ResponseErrorStatus(http.StatusInternalServerError, "could not establish session")
		return
	}
	c.ResponseOk(c.GetSessionUser())
}

func (c *ApiController) requireLocalProvider() bool {
	provider, err := conf.GetAuthProvider()
	if err != nil {
		c.ResponseErrorStatus(http.StatusInternalServerError, err.Error())
		return false
	}
	if provider != "local" {
		c.ResponseErrorStatus(http.StatusNotFound, "local authentication is not enabled")
		return false
	}
	return true
}

func (c *ApiController) decodeAuthRequest(request *authRequest) bool {
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, request); err != nil {
		c.ResponseErrorStatus(http.StatusBadRequest, "invalid JSON request")
		return false
	}
	return true
}

func localAdminIdentity(admin *object.LocalAdmin) *auth.SessionIdentity {
	return &auth.SessionIdentity{
		User: auth.User{
			Id:          "1",
			Owner:       "built-in",
			Name:        admin.Name,
			DisplayName: admin.Name,
			IsAdmin:     true,
			Provider:    "local",
		},
		SessionVersion: admin.SessionVersion,
	}
}

func isTrueLocalRequest(request *http.Request) bool {
	for _, header := range []string{"Forwarded", "X-Forwarded-For", "X-Forwarded-Host", "X-Forwarded-Proto", "X-Real-IP"} {
		if strings.TrimSpace(request.Header.Get(header)) != "" {
			return false
		}
	}
	listener := strings.TrimSpace(conf.GetConfigStringDefault("httpaddr", "127.0.0.1"))
	if !isLoopbackHost(listener) || !isLoopbackHost(requestSourceIP(request)) {
		return false
	}
	host := request.Host
	if parsedHost, _, err := net.SplitHostPort(host); err == nil {
		host = parsedHost
	}
	return isLoopbackHost(host)
}

func requestSourceIP(request *http.Request) string {
	host, _, err := net.SplitHostPort(request.RemoteAddr)
	if err == nil {
		return host
	}
	return request.RemoteAddr
}

func isLoopbackHost(host string) bool {
	host = strings.Trim(strings.TrimSpace(host), "[]")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (l *loginLimiter) isLocked(key string, now time.Time) bool {
	l.Lock()
	defer l.Unlock()
	failure, ok := l.failures[key]
	if !ok {
		return false
	}
	if now.After(failure.LockedUntil) && now.Sub(failure.WindowStart) >= loginAttemptWindow {
		delete(l.failures, key)
		return false
	}
	return now.Before(failure.LockedUntil)
}

func (l *loginLimiter) fail(key string, now time.Time) bool {
	l.Lock()
	defer l.Unlock()
	for existingKey, existing := range l.failures {
		if now.After(existing.LockedUntil) && now.Sub(existing.WindowStart) >= loginAttemptWindow {
			delete(l.failures, existingKey)
		}
	}
	if len(l.failures) >= 10000 {
		var oldestKey string
		var oldest time.Time
		for existingKey, existing := range l.failures {
			if oldestKey == "" || existing.WindowStart.Before(oldest) {
				oldestKey, oldest = existingKey, existing.WindowStart
			}
		}
		delete(l.failures, oldestKey)
	}
	failure := l.failures[key]
	if failure.WindowStart.IsZero() || now.Sub(failure.WindowStart) >= loginAttemptWindow {
		failure = loginFailure{WindowStart: now}
	}
	failure.Count++
	if failure.Count > maxLoginFailures {
		failure.LockedUntil = now.Add(loginAttemptWindow)
	}
	l.failures[key] = failure
	return now.Before(failure.LockedUntil)
}

func (l *loginLimiter) clear(key string) {
	l.Lock()
	delete(l.failures, key)
	l.Unlock()
}
