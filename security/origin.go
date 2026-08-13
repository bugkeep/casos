package security

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/casosorg/casos/conf"
)

func IsAllowedOrigin(origin string, request *http.Request) bool {
	if strings.TrimSpace(origin) == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return false
	}
	requestScheme := "http"
	if request.TLS != nil {
		requestScheme = "https"
	}
	if strings.EqualFold(parsed.Scheme, requestScheme) && strings.EqualFold(parsed.Host, request.Host) {
		return true
	}
	allowedOrigins := conf.GetConfigString("allowedOrigins")
	if strings.TrimSpace(allowedOrigins) == "" && conf.GetConfigString("runmode") == "dev" {
		allowedOrigins = "http://localhost:8001"
	}
	for _, allowed := range strings.Split(allowedOrigins, ",") {
		if strings.EqualFold(strings.TrimRight(strings.TrimSpace(allowed), "/"), strings.TrimRight(origin, "/")) {
			return true
		}
	}
	return false
}
