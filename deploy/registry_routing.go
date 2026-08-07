package deploy

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

type registryEgressRoute string

const (
	registryEgressDirect                  registryEgressRoute = "direct"
	registryEgressProxy                   registryEgressRoute = "proxy"
	containerdKubernetesVerificationTag                       = "3.10.1"
	containerdKubernetesVerificationImage                     = "registry.k8s.io/pause:" + containerdKubernetesVerificationTag
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

func (s registryRoutingSelection) directHosts() []string {
	hosts := make([]string, 0, 16)
	seen := make(map[string]struct{}, 16)
	for _, decision := range []registryRouteDecision{s.DockerHub, s.Kubernetes} {
		if decision.CanonicalRequired && decision.CanonicalRoute == registryEgressDirect {
			for _, host := range containerdCanonicalRouteHosts(decision) {
				hosts = appendUniqueHost(hosts, seen, host)
			}
		}
		if decision.MirrorEnabled && decision.MirrorRoute == registryEgressDirect {
			for _, host := range containerdMirrorRouteHosts(decision) {
				hosts = appendUniqueHost(hosts, seen, host)
			}
		}
	}
	return hosts
}

func containerdCanonicalRouteHosts(decision registryRouteDecision) []string {
	hosts := []string{decision.RegistryHost}
	switch decision.RegistryNamespace {
	case "docker.io":
		hosts = append(hosts, "auth.docker.io", ".docker.io", "production.cloudflare.docker.com", "production.cloudfront.docker.com", ".cloudflarestorage.com")
	case "registry.k8s.io":
		hosts = append(hosts, ".pkg.dev", ".googleusercontent.com")
	}
	return hosts
}

func containerdMirrorRouteHosts(decision registryRouteDecision) []string {
	hosts := []string{decision.MirrorHost}
	switch decision.RegistryNamespace {
	case "docker.io":
		hosts = append(hosts, ".1ms.run")
	case "registry.k8s.io":
		hosts = append(hosts, ".aliyuncs.com")
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
	name                         string
	namespace                    string
	registryHost                 string
	canonicalURL                 string
	canonicalDependencyProbeURLs []string
	mirrorHost                   string
	mirrorURL                    string
}

var registryRouteTargets = [...]registryRouteTarget{
	{
		name:         "Docker Hub",
		namespace:    "docker.io",
		registryHost: "registry-1.docker.io",
		canonicalURL: "https://registry-1.docker.io/v2/",
		canonicalDependencyProbeURLs: []string{
			"https://auth.docker.io/token?service=registry.docker.io&scope=repository%3Alibrary%2Fbusybox%3Apull",
		},
		mirrorHost: "docker.1ms.run",
		mirrorURL:  "https://docker.1ms.run/v2/",
	},
	{
		name:         "registry.k8s.io",
		namespace:    "registry.k8s.io",
		registryHost: "registry.k8s.io",
		canonicalURL: "https://registry.k8s.io/v2/pause/manifests/" + containerdKubernetesVerificationTag,
		mirrorHost:   "registry.aliyuncs.com",
		mirrorURL:    "https://registry.aliyuncs.com/v2/google_containers/pause/manifests/" + containerdKubernetesVerificationTag,
	},
}

func probeRegistryPath(ctx context.Context, runner registryMirrorFileRunner, targetURL, proxyURL string) (bool, string, error) {
	routeArgs := "--noproxy '*'"
	proxyEnvironment := ""
	if proxyURL != "" {
		routeArgs = "--noproxy ''"
		proxyEnvironment = `HTTPS_PROXY="$curl_proxy_url" https_proxy="$curl_proxy_url" ALL_PROXY="$curl_proxy_url" all_proxy="$curl_proxy_url" NO_PROXY= no_proxy= `
	}
	commandPrefix := ""
	if proxyURL != "" {
		commandPrefix = `IFS= read -r proxy_url
test -n "$proxy_url"
curl_proxy_url=$proxy_url
case "$curl_proxy_url" in
  socks5://*) curl_proxy_url="socks5h://${proxy_url#socks5://}" ;;
esac
`
	}
	command := commandPrefix + fmt.Sprintf(`attempt=1
status=000
rc=0
while [ "$attempt" -le 3 ]; do
  status=$(%scurl -sS --location --output /dev/null --write-out '%%{http_code}' --connect-timeout 5 --max-time 15 --header 'Accept: application/vnd.oci.image.index.v1+json, application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.list.v2+json, application/vnd.docker.distribution.manifest.v2+json' %s %s 2>/dev/null)
  rc=$?
  case "$rc:$status" in
    0:2??|0:401) printf 'reachable:%%s' "$status"; exit 0 ;;
  esac
  [ "$attempt" -eq 3 ] && break
  attempt=$((attempt + 1))
  sleep 1
done
printf 'unreachable:curl:%%s:http:%%s' "$rc" "${status:-000}"`, proxyEnvironment, routeArgs, shellSingleQuote(targetURL))
	var output string
	var err error
	if proxyURL == "" {
		output, err = runner.RunRootContext(ctx, command)
	} else {
		output, err = runner.RunRootInputSensitiveContext(ctx, command, proxyURL+"\n")
	}
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
	if strings.HasPrefix(result, "unreachable:curl:") {
		diagnostic := strings.TrimPrefix(result, "unreachable:curl:")
		curlExit, httpStatus, ok := strings.Cut(diagnostic, ":http:")
		if !ok || curlExit == "" || len(httpStatus) != 3 {
			return false, "", fmt.Errorf("unexpected probe result %q", result)
		}
		if _, err := strconv.Atoi(curlExit); err != nil {
			return false, "", fmt.Errorf("unexpected probe result %q", result)
		}
		if _, err := strconv.Atoi(httpStatus); err != nil {
			return false, "", fmt.Errorf("unexpected probe result %q", result)
		}
		return false, fmt.Sprintf("curl exit %s, HTTP status %s", curlExit, httpStatus), nil
	}
	return false, "", fmt.Errorf("unexpected probe result %q", result)
}

func probeRegistryRoute(
	ctx context.Context,
	runner registryMirrorFileRunner,
	primaryURL string,
	dependencyURLs []string,
	proxyURL string,
) (bool, string, error) {
	reachable, detail, err := probeRegistryPath(ctx, runner, primaryURL, proxyURL)
	if err != nil || !reachable {
		return reachable, detail, err
	}
	for _, dependencyURL := range dependencyURLs {
		dependencyReachable, dependencyDetail, err := probeRegistryPath(ctx, runner, dependencyURL, proxyURL)
		if err != nil {
			return false, "", err
		}
		if !dependencyReachable || !strings.HasPrefix(dependencyDetail, "HTTP status 2") {
			return false, fmt.Sprintf("dependency %s is unreachable (%s)", dependencyURL, dependencyDetail), nil
		}
	}
	return true, detail, nil
}

func registryProbeHTTPStatusReachable(status int) bool {
	return status >= 200 && status < 300 || status == 401
}
