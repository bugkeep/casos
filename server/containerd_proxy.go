package server

import (
	"strings"

	"github.com/casosorg/casos/proxy"
)

func normalizeContainerdProxy(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	return proxy.NormalizeSocks5ProxyAddress(value)
}
