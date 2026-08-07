package deploy

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/casosorg/casos/server"
)

type workerNetworkFacts struct {
	WSL              bool
	WSLGateway       string
	SSHClientAddress string
}

const (
	workerProxySourceConfigured = "configured"
	workerProxySourceWSLGateway = "wsl-gateway"
	workerProxySourceSSHClient  = "ssh-client"
)

type workerProxyCandidate struct {
	URL    string
	Source string
}

type workerEgressSelection struct {
	ProxyURL    string
	ProxySource string
	Routing     registryRoutingSelection
}

type workerEgressPolicy struct {
	ProxySeed            string
	MirrorMode           server.RegistryMirrorMode
	ForceMirrorNamespace string
}

func buildWorkerEgressPolicy(config Config) workerEgressPolicy {
	return workerEgressPolicy{
		ProxySeed:  strings.TrimSpace(config.WorkerSocks5Proxy),
		MirrorMode: config.RegistryMirrorMode,
	}
}

func (p workerEgressPolicy) withForcedMirrorNamespace(namespace string) (workerEgressPolicy, error) {
	if p.MirrorMode != server.RegistryMirrorModeAuto || !isForceableMirrorNamespace(namespace) {
		return workerEgressPolicy{}, fmt.Errorf("cannot force mirror namespace %q in mode %q", namespace, p.MirrorMode)
	}
	p.ForceMirrorNamespace = namespace
	return p, nil
}

func (p workerEgressPolicy) mirrorModeForNamespace(namespace string) (server.RegistryMirrorMode, error) {
	if p.ForceMirrorNamespace == "" {
		return p.MirrorMode, nil
	}
	if p.MirrorMode != server.RegistryMirrorModeAuto || !isForceableMirrorNamespace(p.ForceMirrorNamespace) {
		return "", fmt.Errorf("invalid forced mirror namespace %q in mode %q", p.ForceMirrorNamespace, p.MirrorMode)
	}
	if namespace == p.ForceMirrorNamespace {
		return server.RegistryMirrorModeAlways, nil
	}
	return p.MirrorMode, nil
}

func isForceableMirrorNamespace(namespace string) bool {
	return namespace == "docker.io" || namespace == "registry.k8s.io"
}

type registryPathReachability struct {
	CanonicalDirect bool
	MirrorDirect    bool
	CanonicalProxy  map[string]bool
	MirrorProxy     map[string]bool
}

type registryReachability map[string]registryPathReachability

func discoverWorkerNetworkFacts(ctx context.Context, runner *NodeDeploySSHRunner, wsl bool) (workerNetworkFacts, error) {
	facts := workerNetworkFacts{WSL: wsl}
	if runner == nil {
		return facts, fmt.Errorf("discover worker network facts: SSH runner is required")
	}

	command := `# discover-worker-network-facts
ssh_client=$(printf '%s\n' "$SSH_CONNECTION" | awk '{ print $1; exit }')
wsl_gateway=
if [ -f /proc/sys/fs/binfmt_misc/WSLInterop ] || grep -qi microsoft /proc/version; then
  wsl_gateway=$(ip route | awk '$1 == "default" { print $3; exit }')
fi
printf 'ssh-client=%s\nwsl-gateway=%s\n' "$ssh_client" "$wsl_gateway"`
	output, err := runner.RunContext(ctx, command)
	if err != nil {
		return facts, fmt.Errorf("discover worker network facts: %w", err)
	}
	sshClient, wslGateway, err := parseWorkerNetworkFacts(output)
	if err != nil {
		return facts, err
	}
	facts.SSHClientAddress = sshClient
	if facts.WSL {
		facts.WSLGateway = wslGateway
	}
	return facts, nil
}

func parseWorkerNetworkFacts(output string) (string, string, error) {
	values := make(map[string]string, 2)
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		values[key] = strings.TrimSpace(value)
	}
	sshClient := values["ssh-client"]
	wslGateway := values["wsl-gateway"]
	for name, address := range map[string]string{"SSH client": sshClient, "WSL gateway": wslGateway} {
		if address != "" && net.ParseIP(address) == nil {
			return "", "", fmt.Errorf("discover worker network facts: invalid %s address", name)
		}
	}
	return sshClient, wslGateway, nil
}

func buildWorkerProxyCandidates(proxyAddress string, facts workerNetworkFacts) ([]workerProxyCandidate, error) {
	proxyAddress = strings.TrimSpace(proxyAddress)
	if proxyAddress == "" {
		return nil, nil
	}
	parsed, err := url.Parse(proxyAddress)
	if err != nil || (parsed.Scheme != "socks5" && parsed.Scheme != "socks5h") || parsed.Hostname() == "" || parsed.Port() == "" {
		return nil, fmt.Errorf("build worker proxy candidates: invalid SOCKS5 proxy address")
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil || port < 1 || port > 65535 {
		return nil, fmt.Errorf("build worker proxy candidates: invalid SOCKS5 proxy address")
	}
	if parsed.Path != "" && parsed.Path != "/" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, fmt.Errorf("build worker proxy candidates: invalid SOCKS5 proxy address")
	}
	// Older containerd 1.x builds reject the socks5h alias. Their SOCKS5
	// transport still forwards hostnames to the proxy for remote resolution.
	parsed.Scheme = "socks5"
	parsed.Path = ""

	candidates := make([]workerProxyCandidate, 0, 3)
	seen := make(map[string]struct{}, 3)
	appendCandidate := func(host, source string) {
		host = strings.TrimSpace(host)
		if host == "" {
			return
		}
		candidateURL := *parsed
		candidateURL.Host = net.JoinHostPort(host, parsed.Port())
		value := candidateURL.String()
		if _, ok := seen[value]; ok {
			return
		}
		seen[value] = struct{}{}
		candidates = append(candidates, workerProxyCandidate{URL: value, Source: source})
	}

	appendCandidate(parsed.Hostname(), workerProxySourceConfigured)
	if !isLoopbackProxyHost(parsed.Hostname()) {
		return candidates, nil
	}
	if facts.WSL {
		appendCandidate(facts.WSLGateway, workerProxySourceWSLGateway)
	}
	appendCandidate(facts.SSHClientAddress, workerProxySourceSSHClient)
	return candidates, nil
}

func isLoopbackProxyHost(host string) bool {
	host = strings.Trim(strings.TrimSpace(host), "[]")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func selectRegistryRoutingForPolicy(policy workerEgressPolicy, proxyURL string, paths registryReachability) (registryRoutingSelection, error) {
	selection := registryRoutingSelection{}
	for _, target := range registryRouteTargets {
		mode, err := policy.mirrorModeForNamespace(target.namespace)
		if err != nil {
			return registryRoutingSelection{}, err
		}
		decision, err := selectRegistryRoute(mode, proxyURL, target, paths[target.namespace])
		if err != nil {
			return registryRoutingSelection{}, fmt.Errorf("%s: %w", target.name, err)
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

func selectRegistryRoute(mode server.RegistryMirrorMode, proxyURL string, target registryRouteTarget, paths registryPathReachability) (registryRouteDecision, error) {
	decision := registryRouteDecision{
		RegistryNamespace: target.namespace,
		RegistryHost:      target.registryHost,
		MirrorHost:        target.mirrorHost,
		CanonicalRoute:    registryEgressDirect,
	}
	canonicalProxy := proxyURL != "" && paths.CanonicalProxy[proxyURL]
	mirrorProxy := proxyURL != "" && paths.MirrorProxy[proxyURL]

	switch mode {
	case server.RegistryMirrorModeAlways:
		decision.MirrorEnabled = true
		switch {
		case paths.MirrorDirect:
			decision.MirrorRoute = registryEgressDirect
		case mirrorProxy:
			decision.MirrorRoute = registryEgressProxy
		default:
			return registryRouteDecision{}, fmt.Errorf("required mirror is unreachable")
		}
	case server.RegistryMirrorModeNever:
		decision.CanonicalRequired = true
		switch {
		case paths.CanonicalDirect:
			decision.CanonicalRoute = registryEgressDirect
		case canonicalProxy:
			decision.CanonicalRoute = registryEgressProxy
		default:
			return registryRouteDecision{}, fmt.Errorf("canonical registry is unreachable")
		}
	case server.RegistryMirrorModeAuto:
		switch {
		case paths.CanonicalDirect:
			decision.CanonicalRequired = true
			decision.CanonicalRoute = registryEgressDirect
		case paths.MirrorDirect:
			decision.MirrorEnabled = true
			decision.MirrorRoute = registryEgressDirect
		case canonicalProxy:
			decision.CanonicalRequired = true
			decision.CanonicalRoute = registryEgressProxy
		case mirrorProxy:
			decision.MirrorEnabled = true
			decision.MirrorRoute = registryEgressProxy
		default:
			return registryRouteDecision{}, fmt.Errorf("canonical registry and mirror are unreachable")
		}
	default:
		return registryRouteDecision{}, fmt.Errorf("unsupported registry mirror mode %q", mode)
	}
	return decision, nil
}

func (d *NodeDeployer) resolveWorkerEgress(ctx context.Context, runner registryMirrorFileRunner, policy workerEgressPolicy, factsProvider func() (workerNetworkFacts, error)) (workerEgressSelection, error) {
	paths := make(registryReachability, len(registryRouteTargets))
	for _, target := range registryRouteTargets {
		mode, err := policy.mirrorModeForNamespace(target.namespace)
		if err != nil {
			return workerEgressSelection{}, err
		}
		path := registryPathReachability{
			CanonicalProxy: make(map[string]bool),
			MirrorProxy:    make(map[string]bool),
		}
		if mode != server.RegistryMirrorModeAlways {
			path.CanonicalDirect, _, err = probeRegistryRoute(ctx, runner, target.canonicalURL, target.canonicalDependencyProbeURLs, "")
			if err != nil {
				return workerEgressSelection{}, fmt.Errorf("probe %s canonical registry directly from worker: %w", target.name, err)
			}
		}
		if mode != server.RegistryMirrorModeNever && (mode != server.RegistryMirrorModeAuto || !path.CanonicalDirect) {
			path.MirrorDirect, _, err = probeRegistryPath(ctx, runner, target.mirrorURL, "")
			if err != nil {
				return workerEgressSelection{}, fmt.Errorf("probe %s mirror directly from worker: %w", target.name, err)
			}
		}
		paths[target.namespace] = path
	}

	routing, directErr := selectRegistryRoutingForPolicy(policy, "", paths)
	if directErr == nil {
		return workerEgressSelection{Routing: routing}, nil
	}
	if strings.TrimSpace(policy.ProxySeed) == "" {
		return workerEgressSelection{}, fmt.Errorf("direct network does not provide complete registry routing: %w", directErr)
	}
	if factsProvider == nil {
		return workerEgressSelection{}, fmt.Errorf("discover worker proxy candidates: network facts provider is required")
	}
	facts, err := factsProvider()
	if err != nil {
		return workerEgressSelection{}, fmt.Errorf("discover worker proxy candidates: %w", err)
	}
	candidates, err := buildWorkerProxyCandidates(policy.ProxySeed, facts)
	if err != nil {
		return workerEgressSelection{}, err
	}

	for _, candidate := range candidates {
		for _, target := range registryRouteTargets {
			mode, err := policy.mirrorModeForNamespace(target.namespace)
			if err != nil {
				return workerEgressSelection{}, err
			}
			path := paths[target.namespace]
			if _, err := selectRegistryRoute(mode, "", target, path); err == nil {
				continue
			}

			switch mode {
			case server.RegistryMirrorModeNever:
				path.CanonicalProxy[candidate.URL], _, err = probeRegistryRoute(ctx, runner, target.canonicalURL, target.canonicalDependencyProbeURLs, candidate.URL)
				if err != nil {
					return workerEgressSelection{}, fmt.Errorf("probe %s canonical registry through worker proxy candidate: %w", target.name, err)
				}
			case server.RegistryMirrorModeAlways:
				path.MirrorProxy[candidate.URL], _, err = probeRegistryPath(ctx, runner, target.mirrorURL, candidate.URL)
				if err != nil {
					return workerEgressSelection{}, fmt.Errorf("probe %s mirror through worker proxy candidate: %w", target.name, err)
				}
			case server.RegistryMirrorModeAuto:
				path.CanonicalProxy[candidate.URL], _, err = probeRegistryRoute(ctx, runner, target.canonicalURL, target.canonicalDependencyProbeURLs, candidate.URL)
				if err != nil {
					return workerEgressSelection{}, fmt.Errorf("probe %s canonical registry through worker proxy candidate: %w", target.name, err)
				}
				if !path.CanonicalProxy[candidate.URL] {
					path.MirrorProxy[candidate.URL], _, err = probeRegistryPath(ctx, runner, target.mirrorURL, candidate.URL)
					if err != nil {
						return workerEgressSelection{}, fmt.Errorf("probe %s mirror through worker proxy candidate: %w", target.name, err)
					}
				}
			default:
				return workerEgressSelection{}, fmt.Errorf("unsupported registry mirror mode %q", mode)
			}
			paths[target.namespace] = path
		}

		routing, err := selectRegistryRoutingForPolicy(policy, candidate.URL, paths)
		if err == nil {
			return workerEgressSelection{
				ProxyURL:    candidate.URL,
				ProxySource: candidate.Source,
				Routing:     routing,
			}, nil
		}
	}
	return workerEgressSelection{}, fmt.Errorf("no proxy candidate completes all registry routes missing from the direct network")
}
