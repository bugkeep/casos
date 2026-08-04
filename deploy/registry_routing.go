package deploy

import (
	"context"
	"fmt"
	"strings"

	"github.com/casosorg/casos/server"
)

type RegistryEgressRoute string

const (
	RegistryEgressDirect RegistryEgressRoute = "direct"
	RegistryEgressProxy  RegistryEgressRoute = "proxy"
)

type RegistryRouteDecision struct {
	RegistryNamespace string
	RegistryHost      string
	MirrorHost        string
	MirrorEnabled     bool
	CanonicalRoute    RegistryEgressRoute
	MirrorRoute       RegistryEgressRoute
}

type RegistryRoutingSelection struct {
	DockerHub  RegistryRouteDecision
	Kubernetes RegistryRouteDecision
}

func (s RegistryRoutingSelection) DirectHosts() []string {
	hosts := make([]string, 0, 4)
	seen := make(map[string]struct{}, 4)
	for _, decision := range []RegistryRouteDecision{s.DockerHub, s.Kubernetes} {
		if decision.CanonicalRoute == RegistryEgressDirect {
			hosts = appendUniqueHost(hosts, seen, decision.RegistryHost)
		}
		if decision.MirrorEnabled && decision.MirrorRoute == RegistryEgressDirect {
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

var registryRouteTargets = []registryRouteTarget{
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

func (d *NodeDeployer) resolveRegistryRouting(ctx context.Context, runner registryMirrorFileRunner) (RegistryRoutingSelection, error) {
	switch d.config.RegistryMirrorMode {
	case server.RegistryMirrorModeAlways:
		d.logStep(nodeDeployPhaseConfiguring, "Registry mirror mode always: enabling Docker Hub and registry.k8s.io mirrors")
		return staticRegistryRouting(true, d.config.ContainerdProxy != ""), nil
	case server.RegistryMirrorModeNever:
		d.logStep(nodeDeployPhaseConfiguring, "Registry mirror mode never: disabling Docker Hub and registry.k8s.io mirrors")
		return staticRegistryRouting(false, d.config.ContainerdProxy != ""), nil
	case server.RegistryMirrorModeAuto:
		d.logStep(nodeDeployPhaseConfiguring, "Registry mirror mode auto: probing canonical registries, mirrors, and configured proxy from the target worker")
	default:
		return RegistryRoutingSelection{}, fmt.Errorf("unsupported registry mirror mode %q", d.config.RegistryMirrorMode)
	}

	decisions := make([]RegistryRouteDecision, 0, len(registryRouteTargets))
	for _, target := range registryRouteTargets {
		decision, err := d.resolveRegistryRoute(ctx, runner, target)
		if err != nil {
			return RegistryRoutingSelection{}, err
		}
		decisions = append(decisions, decision)
	}
	return RegistryRoutingSelection{DockerHub: decisions[0], Kubernetes: decisions[1]}, nil
}

func staticRegistryRouting(mirrorsEnabled, proxyConfigured bool) RegistryRoutingSelection {
	decisions := make([]RegistryRouteDecision, 0, len(registryRouteTargets))
	for _, target := range registryRouteTargets {
		canonicalRoute := RegistryEgressDirect
		if proxyConfigured {
			canonicalRoute = RegistryEgressProxy
		}
		decision := RegistryRouteDecision{
			RegistryNamespace: target.namespace,
			RegistryHost:      target.registryHost,
			MirrorHost:        target.mirrorHost,
			MirrorEnabled:     mirrorsEnabled,
			CanonicalRoute:    canonicalRoute,
		}
		if mirrorsEnabled {
			decision.MirrorRoute = RegistryEgressDirect
		}
		decisions = append(decisions, decision)
	}
	return RegistryRoutingSelection{DockerHub: decisions[0], Kubernetes: decisions[1]}
}

func (d *NodeDeployer) resolveRegistryRoute(ctx context.Context, runner registryMirrorFileRunner, target registryRouteTarget) (RegistryRouteDecision, error) {
	directReachable, directDetail, err := probeRegistryPath(ctx, runner, target.canonicalURL, "")
	if err != nil {
		return RegistryRouteDecision{}, fmt.Errorf("probe %s canonical registry directly from worker: %w", target.name, err)
	}
	if directReachable {
		d.logStep(nodeDeployPhaseConfiguring, fmt.Sprintf("%s canonical registry is directly reachable (%s); mirror disabled", target.name, directDetail))
		return RegistryRouteDecision{
			RegistryNamespace: target.namespace,
			RegistryHost:      target.registryHost,
			MirrorHost:        target.mirrorHost,
			CanonicalRoute:    RegistryEgressDirect,
		}, nil
	}

	mirrorReachable, mirrorDetail, err := probeRegistryPath(ctx, runner, target.mirrorURL, "")
	if err != nil {
		return RegistryRouteDecision{}, fmt.Errorf("probe %s mirror directly from worker: %w", target.name, err)
	}
	proxyReachable := false
	proxyDetail := "node proxy is not configured"
	if d.config.ContainerdProxy != "" {
		proxyReachable, proxyDetail, err = probeRegistryPath(ctx, runner, target.canonicalURL, d.config.ContainerdProxy)
		if err != nil {
			return RegistryRouteDecision{}, fmt.Errorf("probe %s canonical registry through node proxy: %w", target.name, err)
		}
	}
	if !proxyReachable {
		return RegistryRouteDecision{}, fmt.Errorf("no complete pull path for %s: canonical direct unavailable (%s), canonical proxy unavailable (%s), mirror direct reachable=%t (%s)", target.name, directDetail, proxyDetail, mirrorReachable, mirrorDetail)
	}

	decision := RegistryRouteDecision{
		RegistryNamespace: target.namespace,
		RegistryHost:      target.registryHost,
		MirrorHost:        target.mirrorHost,
		CanonicalRoute:    RegistryEgressProxy,
	}
	if mirrorReachable {
		decision.MirrorEnabled = true
		decision.MirrorRoute = RegistryEgressDirect
		d.logStep(nodeDeployPhaseConfiguring, fmt.Sprintf("%s canonical registry will use the node proxy (%s); directly reachable mirror enabled (%s)", target.name, proxyDetail, mirrorDetail))
		return decision, nil
	}
	d.logStep(nodeDeployPhaseConfiguring, fmt.Sprintf("%s mirror is directly unreachable (%s); mirror disabled and canonical registry will use the node proxy (%s)", target.name, mirrorDetail, proxyDetail))
	return decision, nil
}

func probeRegistryPath(ctx context.Context, runner registryMirrorFileRunner, targetURL, proxyURL string) (bool, string, error) {
	routeArgs := "--noproxy '*'"
	if proxyURL != "" {
		routeArgs = "--noproxy '' --proxy " + shellSingleQuote(proxyURL)
	}
	command := fmt.Sprintf(`if curl -sS --location --output /dev/null --connect-timeout 2 --max-time 8 %s %s 2>/dev/null; then
  printf reachable
else
  rc=$?
  printf 'unreachable:%%s' "$rc"
fi`, routeArgs, shellSingleQuote(targetURL))
	output, err := runner.RunRootContext(ctx, command)
	if err != nil {
		return false, "", err
	}
	result := strings.TrimSpace(output)
	if result == "reachable" {
		return true, "HTTP response received", nil
	}
	if strings.HasPrefix(result, "unreachable:") {
		return false, "curl exit " + strings.TrimPrefix(result, "unreachable:"), nil
	}
	return false, "", fmt.Errorf("unexpected probe result %q", result)
}
