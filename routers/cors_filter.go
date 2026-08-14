package routers

import (
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/beego/beego/context"
)

func CorsFilter(ctx *context.Context) {
	origin := ctx.Request.Header.Get("Origin")
	if origin == "" {
		return
	}
	if !isAllowedOriginRequest(origin, ctx.Request) {
		ctx.Abort(http.StatusForbidden, "")
		return
	}

	ctx.ResponseWriter.Header().Add("Vary", "Origin")
	ctx.ResponseWriter.Header().Set("Access-Control-Allow-Origin", origin)
	ctx.ResponseWriter.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, PUT, PATCH, OPTIONS")
	ctx.ResponseWriter.Header().Set("Access-Control-Allow-Headers", "Origin, X-Requested-With, Content-Type, Accept")
	ctx.ResponseWriter.Header().Set("Access-Control-Allow-Credentials", "true")
	if ctx.Request.Method == http.MethodOptions {
		ctx.Abort(http.StatusNoContent, "")
	}
}

func isAllowedOriginRequest(origin string, request *http.Request) bool {
	parsedOrigin, err := url.Parse(origin)
	if err != nil || parsedOrigin.Scheme == "" || parsedOrigin.Host == "" ||
		parsedOrigin.User != nil || (parsedOrigin.Path != "" && parsedOrigin.Path != "/") ||
		parsedOrigin.RawQuery != "" || parsedOrigin.Fragment != "" {
		return false
	}

	requestScheme, requestHost, ok := requestOrigin(request)
	if !ok {
		return false
	}
	if sameOrigin(parsedOrigin.Scheme, parsedOrigin.Host, requestScheme, requestHost) {
		return true
	}

	return strings.EqualFold(parsedOrigin.Scheme, "http") &&
		strings.EqualFold(requestScheme, "http") &&
		isLoopbackDevelopmentPair(parsedOrigin.Host, requestHost)
}

func sameOrigin(leftScheme, leftHost, rightScheme, rightHost string) bool {
	leftHostname, leftPort, leftOK := canonicalOriginHost(leftScheme, leftHost)
	rightHostname, rightPort, rightOK := canonicalOriginHost(rightScheme, rightHost)
	return leftOK && rightOK && strings.EqualFold(leftScheme, rightScheme) &&
		strings.EqualFold(leftHostname, rightHostname) && leftPort == rightPort
}

func canonicalOriginHost(scheme, host string) (string, string, bool) {
	parsed, err := url.Parse("//" + host)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.Path != "" ||
		parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", "", false
	}
	port := parsed.Port()
	if port == "" {
		switch strings.ToLower(scheme) {
		case "http":
			port = "80"
		case "https":
			port = "443"
		default:
			return "", "", false
		}
	}
	return parsed.Hostname(), port, parsed.Hostname() != ""
}

func isLoopbackDevelopmentPair(originHost, requestHost string) bool {
	return isLoopbackHostPort(originHost, "8001") && isLoopbackHostPort(requestHost, "9000") ||
		isLoopbackHostPort(originHost, "9000") && isLoopbackHostPort(requestHost, "8001")
}

func isLoopbackHostPort(hostPort, expectedPort string) bool {
	parsed, err := url.Parse("//" + hostPort)
	if err != nil || parsed.Port() != expectedPort {
		return false
	}
	hostname := parsed.Hostname()
	if strings.EqualFold(hostname, "localhost") {
		return true
	}
	ip := net.ParseIP(hostname)
	return ip != nil && ip.IsLoopback()
}

func requestOrigin(request *http.Request) (string, string, bool) {
	requestScheme := request.URL.Scheme
	if requestScheme == "" {
		requestScheme = "http"
		if request.TLS != nil {
			requestScheme = "https"
		}
	}
	requestHost := request.Host

	if forwarded := strings.TrimSpace(request.Header.Get("Forwarded")); forwarded != "" {
		proto, host, ok := parseForwardedOrigin(forwarded)
		if !ok {
			return "", "", false
		}
		if proto != "" {
			requestScheme = proto
		}
		if host != "" {
			requestHost = host
		}
	} else {
		proto := strings.ToLower(strings.TrimSpace(request.Header.Get("X-Forwarded-Proto")))
		host := strings.TrimSpace(request.Header.Get("X-Forwarded-Host"))
		if strings.Contains(proto, ",") || strings.Contains(host, ",") {
			return "", "", false
		}
		if proto != "" {
			requestScheme = proto
		}
		if host != "" {
			requestHost = host
		}
	}

	if requestScheme != "http" && requestScheme != "https" {
		return "", "", false
	}
	if !validOriginHost(requestHost) {
		return "", "", false
	}
	return requestScheme, requestHost, true
}

func parseForwardedOrigin(value string) (string, string, bool) {
	if strings.Contains(value, ",") {
		return "", "", false
	}
	var proto, host string
	for _, parameter := range strings.Split(value, ";") {
		key, rawValue, found := strings.Cut(strings.TrimSpace(parameter), "=")
		if !found {
			return "", "", false
		}
		parsedValue := strings.TrimSpace(rawValue)
		if strings.HasPrefix(parsedValue, `"`) {
			unquoted, err := strconv.Unquote(parsedValue)
			if err != nil {
				return "", "", false
			}
			parsedValue = unquoted
		}
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "proto":
			if proto != "" {
				return "", "", false
			}
			proto = strings.ToLower(parsedValue)
		case "host":
			if host != "" {
				return "", "", false
			}
			host = parsedValue
		}
	}
	return proto, host, true
}

func validOriginHost(host string) bool {
	parsed, err := url.Parse("//" + host)
	return err == nil && parsed.Host != "" && parsed.User == nil && parsed.Path == "" &&
		parsed.RawQuery == "" && parsed.Fragment == ""
}
