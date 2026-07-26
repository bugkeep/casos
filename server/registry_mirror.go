package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/casosorg/casos/conf"
)

// imageRegistryMirror modes accepted in app.conf.
const (
	registryMirrorModeAuto   = "auto"
	registryMirrorModeAlways = "always"
	registryMirrorModeNever  = "never"
)

// registryProbeURL is probed once (mode "auto") to decide whether the canonical
// registries are reachable. Any HTTP response, including the 401 that Docker
// Hub returns for anonymous requests, counts as reachable; only a connection
// failure or timeout marks the environment as restricted.
const (
	registryProbeURL     = "https://registry-1.docker.io/v2/"
	registryProbeTimeout = 4 * time.Second
)

var (
	registryProbeOnce       sync.Once
	registryProbeRestricted bool
)

// resolveRegistryMirror reads imageRegistryMirror from app.conf and decides
// whether built-in image pulls should be routed through the configured
// registry mirrors. Mode "auto" (the default) probes the canonical registry
// directly instead of guessing from timezone, locale, or IP geolocation; the
// result is cached for the lifetime of the process. An explicit
// "always"/"never" skips the probe entirely.
func resolveRegistryMirror() (bool, error) {
	mode := strings.ToLower(strings.TrimSpace(conf.GetConfigStringDefault("imageRegistryMirror", registryMirrorModeAuto)))
	switch mode {
	case registryMirrorModeAlways:
		return true, nil
	case registryMirrorModeNever:
		return false, nil
	case registryMirrorModeAuto:
		return canonicalRegistryRestricted(), nil
	default:
		return false, fmt.Errorf("invalid imageRegistryMirror %q in app.conf: expected auto, always, or never", mode)
	}
}

// canonicalRegistryRestricted reports whether the canonical Docker registry is
// unreachable from this host. The probe runs at most once per process.
func canonicalRegistryRestricted() bool {
	registryProbeOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), registryProbeTimeout)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodHead, registryProbeURL, nil)
		if err != nil {
			registryProbeRestricted = false
			return
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			registryProbeRestricted = true
			return
		}
		resp.Body.Close()
		registryProbeRestricted = false
	})
	return registryProbeRestricted
}
