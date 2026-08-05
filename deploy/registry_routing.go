package deploy

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/casosorg/casos/server"
	"golang.org/x/net/http/httpproxy"
)

type registryEgressRoute string

const (
	registryEgressDirect registryEgressRoute = "direct"
	registryEgressProxy  registryEgressRoute = "proxy"
)

type registryRouteDecision struct {
	RegistryNamespace string
	RegistryHost      string
	MirrorHost        string
	MirrorEnabled     bool
	CanonicalRoute    registryEgressRoute
	CanonicalRequired bool
	MirrorRoute       registryEgressRoute
}

type registryRoutingSelection struct {
	DockerHub  registryRouteDecision
	Kubernetes registryRouteDecision
}

func (s registryRoutingSelection) DirectHosts() []string {
	hosts := make([]string, 0, 4)
	seen := make(map[string]struct{}, 4)
	for _, decision := range []registryRouteDecision{s.DockerHub, s.Kubernetes} {
		if decision.CanonicalRoute == registryEgressDirect {
			hosts = appendUniqueHost(hosts, seen, decision.RegistryHost)
		}
		if decision.MirrorEnabled && decision.MirrorRoute == registryEgressDirect {
			hosts = appendUniqueHost(hosts, seen, decision.MirrorHost)
		}
	}
	return hosts
}

func appendUniqueHost(hosts []string, seen map[string]struct{}, host string) []string {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return hosts
	}
	if _, ok := seen[host]; ok {
		return hosts
	}
	seen[host] = struct{}{}
	return append(hosts, host)
}

type registryRouteTarget struct {
	name         string
	namespace    string
	registryHost string
	canonicalURL string
	mirrorHost   string
	mirrorURL    string
}

var registryRouteTargets = [...]registryRouteTarget{
	{
		name:         "Docker Hub",
		namespace:    "docker.io",
		registryHost: "registry-1.docker.io",
		canonicalURL: "https://registry-1.docker.io/v2/",
		mirrorHost:   "docker.1ms.run",
		mirrorURL:    "https://docker.1ms.run/v2/",
	},
	{
		name:         "registry.k8s.io",
		namespace:    "registry.k8s.io",
		registryHost: "registry.k8s.io",
		canonicalURL: "https://registry.k8s.io/v2/",
		mirrorHost:   "registry.aliyuncs.com",
		mirrorURL:    "https://registry.aliyuncs.com/v2/google_containers/",
	},
}

func (d *NodeDeployer) resolveRegistryRouting(ctx context.Context, runner registryMirrorFileRunner) (registryRoutingSelection, error) {
	switch d.config.RegistryMirrorMode {
	case server.RegistryMirrorModeAlways:
		d.logStep(nodeDeployPhaseConfiguring, "Registry mirror mode always: enabling Docker Hub and registry.k8s.io mirrors")
		return staticRegistryRouting(true, d.config.ContainerdProxy != "", d.config.ContainerdNoProxy, false), nil
	case server.RegistryMirrorModeNever:
		d.logStep(nodeDeployPhaseConfiguring, "Registry mirror mode never: disabling Docker Hub and registry.k8s.io mirrors")
		return staticRegistryRouting(false, d.config.ContainerdProxy != "", d.config.ContainerdNoProxy, true), nil
	case server.RegistryMirrorModeAuto:
		d.logStep(nodeDeployPhaseConfiguring, "Registry mirror mode auto: probing canonical registries, mirrors, and configured proxy from the target worker")
	default:
		return registryRoutingSelection{}, fmt.Errorf("unsupported registry mirror mode %q", d.config.RegistryMirrorMode)
	}

	selection := registryRoutingSelection{}
	for _, target := range registryRouteTargets {
		decision, err := d.resolveRegistryRoute(ctx, runner, target)
		if err != nil {
			return registryRoutingSelection{}, err
		}
		switch target.namespace {
		case "docker.io":
			selection.DockerHub = decision
		case "registry.k8s.io":
			selection.Kubernetes = decision
		}
	}
	return selection, nil
}

func staticRegistryRouting(mirrorsEnabled, proxyConfigured bool, noProxy []string, canonicalRequired bool) registryRoutingSelection {
	selection := registryRoutingSelection{}
	for _, target := range registryRouteTargets {
		canonicalRoute := registryEgressDirect
		if proxyConfigured && !registryProxyBypassed(target.registryHost, noProxy) {
			canonicalRoute = registryEgressProxy
		}
		decision := registryRouteDecision{
			RegistryNamespace: target.namespace,
			RegistryHost:      target.registryHost,
			MirrorHost:        target.mirrorHost,
			MirrorEnabled:     mirrorsEnabled,
			CanonicalRoute:    canonicalRoute,
			CanonicalRequired: canonicalRequired,
		}
		if mirrorsEnabled {
			decision.MirrorRoute = registryEgressDirect
		}
		switch target.namespace {
		case "docker.io":
			selection.DockerHub = decision
		case "registry.k8s.io":
			selection.Kubernetes = decision
		}
	}
	return selection
}

func (d *NodeDeployer) resolveRegistryRoute(ctx context.Context, runner registryMirrorFileRunner, target registryRouteTarget) (registryRouteDecision, error) {
	directReachable, directDetail, err := probeRegistryPath(ctx, runner, target.canonicalURL, "")
	if err != nil {
		return registryRouteDecision{}, fmt.Errorf("probe %s canonical registry directly from worker: %w", target.name, err)
	}
	if directReachable {
		d.logStep(nodeDeployPhaseConfiguring, fmt.Sprintf("%s canonical registry is directly reachable (%s); mirror disabled", target.name, directDetail))
		return registryRouteDecision{
			RegistryNamespace: target.namespace,
			RegistryHost:      target.registryHost,
			MirrorHost:        target.mirrorHost,
			CanonicalRoute:    registryEgressDirect,
			CanonicalRequired: true,
		}, nil
	}

	mirrorReachable, mirrorDetail, err := probeRegistryPath(ctx, runner, target.mirrorURL, "")
	if err != nil {
		return registryRouteDecision{}, fmt.Errorf("probe %s mirror directly from worker: %w", target.name, err)
	}
	proxyReachable := false
	proxyDetail := "node proxy is not configured"
	proxyConfigured := d.config.ContainerdProxy != "" && !registryProxyBypassed(target.registryHost, d.config.ContainerdNoProxy)
	if proxyConfigured {
		proxyReachable, proxyDetail, err = probeRegistryPath(ctx, runner, target.canonicalURL, d.config.ContainerdProxy)
		if err != nil {
			return registryRouteDecision{}, fmt.Errorf("probe %s canonical registry through node proxy: %w", target.name, err)
		}
	}
	decision, err := selectRegistryRoute(target, directReachable, mirrorReachable, proxyReachable)
	if err != nil {
		return registryRouteDecision{}, fmt.Errorf("no complete pull path for %s: canonical direct unavailable (%s), canonical proxy unavailable (%s), mirror direct reachable=%t (%s)", target.name, directDetail, proxyDetail, mirrorReachable, mirrorDetail)
	}
	if decision.CanonicalRoute == registryEgressProxy && decision.MirrorEnabled {
		d.logStep(nodeDeployPhaseConfiguring, fmt.Sprintf("%s canonical registry will use the node proxy (%s); directly reachable mirror enabled (%s)", target.name, proxyDetail, mirrorDetail))
	} else if decision.MirrorEnabled {
		d.logStep(nodeDeployPhaseConfiguring, fmt.Sprintf("%s canonical registry is unavailable (%s); directly reachable mirror enabled (%s)", target.name, directDetail, mirrorDetail))
	} else {
		d.logStep(nodeDeployPhaseConfiguring, fmt.Sprintf("%s mirror is directly unreachable (%s); canonical registry will use the node proxy (%s)", target.name, mirrorDetail, proxyDetail))
	}
	return decision, nil
}

func selectRegistryRoute(target registryRouteTarget, directReachable, mirrorReachable, proxyReachable bool) (registryRouteDecision, error) {
	decision := registryRouteDecision{
		RegistryNamespace: target.namespace,
		RegistryHost:      target.registryHost,
		MirrorHost:        target.mirrorHost,
	}
	if directReachable {
		decision.CanonicalRoute = registryEgressDirect
		decision.CanonicalRequired = true
		return decision, nil
	}
	if proxyReachable {
		decision.CanonicalRoute = registryEgressProxy
		decision.CanonicalRequired = true
		if mirrorReachable {
			decision.MirrorEnabled = true
			decision.MirrorRoute = registryEgressDirect
		}
		return decision, nil
	}
	if mirrorReachable {
		decision.CanonicalRoute = registryEgressDirect
		decision.CanonicalRequired = false
		decision.MirrorEnabled = true
		decision.MirrorRoute = registryEgressDirect
		return decision, nil
	}
	return registryRouteDecision{}, fmt.Errorf("no route available")
}

func registryProxyBypassed(host string, noProxy []string) bool {
	if len(noProxy) == 0 {
		return false
	}
	target, err := url.Parse("https://" + host + "/v2/")
	if err != nil {
		return false
	}
	proxyFunc := (&httpproxy.Config{
		HTTPProxy:  "socks5h://proxy.invalid:1",
		HTTPSProxy: "socks5h://proxy.invalid:1",
		NoProxy:    strings.Join(noProxy, ","),
	}).ProxyFunc()
	proxy, err := proxyFunc(target)
	return err == nil && proxy == nil
}

func probeRegistryPath(ctx context.Context, runner registryMirrorFileRunner, targetURL, proxyURL string) (bool, string, error) {
	routeArgs := "--noproxy '*'"
	if proxyURL != "" {
		routeArgs = "--noproxy '' --proxy " + shellSingleQuote(proxyURL)
	}
	command := fmt.Sprintf(`status=$(curl -sS --location --output /dev/null --write-out '%%{http_code}' --connect-timeout 2 --max-time 8 %s %s 2>/dev/null || true)
case "$status" in
  2??|401) printf 'reachable:%%s' "$status" ;;
  000) printf 'unreachable:curl' ;;
  *) printf 'unreachable:http:%%s' "$status" ;;
esac`, routeArgs, shellSingleQuote(targetURL))
	output, err := runner.RunRootContext(ctx, command)
	if err != nil {
		return false, "", err
	}
	result := strings.TrimSpace(output)
	if strings.HasPrefix(result, "reachable:") {
		statusText := strings.TrimPrefix(result, "reachable:")
		status, err := strconv.Atoi(statusText)
		if err != nil || !registryProbeHTTPStatusReachable(status) {
			return false, "invalid successful HTTP status " + statusText, nil
		}
		return true, "HTTP status " + statusText, nil
	}
	if strings.HasPrefix(result, "unreachable:") {
		return false, "curl exit " + strings.TrimPrefix(result, "unreachable:"), nil
	}
	return false, "", fmt.Errorf("unexpected probe result %q", result)
}

func registryProbeHTTPStatusReachable(status int) bool {
	return status >= 200 && status < 300 || status == 401
}
