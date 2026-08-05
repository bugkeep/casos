package server

import (
	"fmt"
	"net/url"
	"strings"
	"unicode"
)

func normalizeContainerdProxy(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if !strings.Contains(value, "://") {
		value = "socks5h://" + value
	}
	if strings.IndexFunc(value, unicode.IsControl) >= 0 {
		return "", fmt.Errorf("proxy URL contains control characters")
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return "", fmt.Errorf("proxy URL is malformed")
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	switch parsed.Scheme {
	case "http", "https", "socks5", "socks5h":
	default:
		return "", fmt.Errorf("proxy URL scheme must be http, https, socks5, or socks5h")
	}
	if parsed.Hostname() == "" {
		return "", fmt.Errorf("proxy URL host is required")
	}
	if (parsed.Scheme == "socks5" || parsed.Scheme == "socks5h") && parsed.Port() == "" {
		return "", fmt.Errorf("SOCKS5 proxy URL port is required")
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return "", fmt.Errorf("proxy URL path is not supported")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" || parsed.RawFragment != "" {
		return "", fmt.Errorf("proxy URL query and fragment are not supported")
	}
	parsed.Path = ""
	return parsed.String(), nil
}

func parseContainerdNoProxy(value string) ([]string, error) {
	seen := make(map[string]struct{})
	entries := make([]string, 0)
	for _, item := range strings.Split(value, ",") {
		item = strings.ToLower(strings.TrimSpace(item))
		if item == "" {
			continue
		}
		if strings.IndexFunc(item, unicode.IsControl) >= 0 || strings.IndexFunc(item, unicode.IsSpace) >= 0 {
			return nil, fmt.Errorf("NO_PROXY entry contains whitespace or control characters")
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		entries = append(entries, item)
	}
	return entries, nil
}
