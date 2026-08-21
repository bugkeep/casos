package proxy

import (
	"os"
	"strings"
)

// EnsureClusterNoProxy keeps in-cluster control-plane traffic out of the
// user's outbound proxy. The API server, admission webhooks and kubelet clients
// all inherit this process environment, and proxying a .svc address typically
// fails as an opaque TLS EOF.
//
// It must run before the first call to http.ProxyFromEnvironment because Go
// caches the parsed proxy environment.
func EnsureClusterNoProxy() {
	entries := []string{
		"localhost",
		"127.0.0.1",
		"::1",
		".svc",
		".cluster.local",
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
	}
	existing := firstNonEmpty(os.Getenv("NO_PROXY"), os.Getenv("no_proxy"))
	seen := map[string]bool{}
	result := make([]string, 0, len(entries)+8)
	for _, value := range append(strings.Split(existing, ","), entries...) {
		value = strings.TrimSpace(value)
		key := strings.ToLower(value)
		if value == "" || seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, value)
	}
	value := strings.Join(result, ",")
	_ = os.Setenv("NO_PROXY", value)
	_ = os.Setenv("no_proxy", value)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
