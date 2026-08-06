package deploy

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"golang.org/x/net/http/httpproxy"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type containerdProxyExpectation struct {
	ProxyURL string
	Routing  registryRoutingSelection
}

const containerdDockerVerificationImage = "docker.io/library/busybox:1.37.0"

type containerdPullImageMarker interface {
	error
	containerdPullImage() string
}

type containerdImagePullVerificationError struct {
	registryNamespace string
	err               error
}

func (e *containerdImagePullVerificationError) Error() string {
	return e.err.Error()
}

func (e *containerdImagePullVerificationError) Unwrap() error {
	return e.err
}

func (e *containerdImagePullVerificationError) RegistryNamespace() string {
	return e.registryNamespace
}

func containerdConfigValidationCommand(verifySystemd bool) string {
	commands := []string{
		"set -e",
		"containerd --config /etc/containerd/config.toml config dump >/dev/null",
	}
	if verifySystemd {
		commands = append(commands, "systemd-analyze verify containerd.service >/dev/null")
	}
	return strings.Join(commands, "\n")
}

func containerdVerificationCommand() string {
	return `set -e
# verify-containerd
systemctl is-active --quiet containerd
ctr --address /run/containerd/containerd.sock plugins ls | awk '$1 == "io.containerd.grpc.v1" && $2 == "cri" && $NF == "ok" { found=1 } END { exit !found }'
`
}

func verifyContainerd(
	ctx context.Context,
	runner interface {
		RunRootContext(context.Context, string) (string, error)
		RunRootSensitiveContext(context.Context, string) (string, error)
		VerifyContainerdImagePullCRIContext(context.Context, string) error
	},
	sandboxImage string,
	expectation containerdProxyExpectation,
) error {
	if err := verifyContainerdProcessEnvironment(ctx, runner, expectation); err != nil {
		return err
	}
	if _, err := runner.RunRootContext(ctx, containerdVerificationCommand()); err != nil {
		return fmt.Errorf("verify containerd service and CRI readiness: %w", err)
	}
	for _, image := range containerdVerificationImages(sandboxImage, expectation.Routing) {
		if err := runner.VerifyContainerdImagePullCRIContext(ctx, image); err != nil {
			verificationErr := fmt.Errorf("verify containerd image route for %s: %w", image, err)
			var pullMarker containerdPullImageMarker
			if errors.As(err, &pullMarker) && containerdPullFailureEligibleForRegistryFallback(err) {
				pullImage := normalizeContainerdPullImage(strings.TrimSpace(pullMarker.containerdPullImage()))
				return &containerdImagePullVerificationError{
					registryNamespace: containerdPullImageRegistryNamespace(pullImage),
					err:               verificationErr,
				}
			}
			return verificationErr
		}
	}
	return nil
}

// containerd commonly reports resolver and content-fetch failures as Unknown.
// A server-side deadline while the caller context is still live can also be a
// registry content timeout. Only these ambiguous statuses are eligible for the
// bounded alternate-source retry; explicit failures are returned unchanged.
func containerdPullFailureEligibleForRegistryFallback(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	pullStatus, ok := status.FromError(err)
	if !ok {
		return false
	}
	switch pullStatus.Code() {
	case codes.Unknown, codes.DeadlineExceeded:
		return true
	default:
		return false
	}
}

func containerdPullImageRegistryNamespace(image string) string {
	namespace, _, _ := strings.Cut(image, "/")
	switch namespace {
	case "docker.io", "registry.k8s.io":
		return namespace
	default:
		return ""
	}
}

func containerdVerificationImages(sandboxImage string, routing registryRoutingSelection) []string {
	sandboxImage = normalizeContainerdPullImage(strings.TrimSpace(sandboxImage))
	images := make([]string, 0, 3)
	seenImages := make(map[string]struct{}, 3)
	appendImage := func(image string) {
		image = normalizeContainerdPullImage(strings.TrimSpace(image))
		if image == "" {
			return
		}
		if _, exists := seenImages[image]; exists {
			return
		}
		seenImages[image] = struct{}{}
		images = append(images, image)
	}
	appendImage(sandboxImage)
	if routing.DockerHub.RegistryNamespace != "" && !strings.HasPrefix(sandboxImage, "docker.io/") {
		appendImage(containerdDockerVerificationImage)
	}
	if routing.Kubernetes.RegistryNamespace != "" && !strings.HasPrefix(sandboxImage, "registry.k8s.io/") {
		appendImage(containerdKubernetesVerificationImage)
	}
	return images
}

func normalizeContainerdPullImage(image string) string {
	if image == "" {
		return ""
	}
	first, remainder, hasSlash := strings.Cut(image, "/")
	if !hasSlash {
		return "docker.io/library/" + image
	}
	if first == "localhost" || strings.ContainsAny(first, ".:") {
		return image
	}
	return "docker.io/" + first + "/" + remainder
}

func containerdProcessEnvironmentCommand() string {
	return `set -e
# inspect-containerd-process-environment
pid=$(systemctl show containerd --property=MainPID --value)
case "$pid" in
  ''|0|*[!0-9]*) echo "containerd does not have a running MainPID" >&2; exit 1 ;;
esac
test -r "/proc/$pid/environ"
base64 -w0 "/proc/$pid/environ"`
}

func verifyContainerdProcessEnvironment(
	ctx context.Context,
	runner interface {
		RunRootSensitiveContext(context.Context, string) (string, error)
	},
	expectation containerdProxyExpectation,
) error {
	output, err := runner.RunRootSensitiveContext(ctx, containerdProcessEnvironmentCommand())
	if err != nil {
		return fmt.Errorf("inspect running containerd proxy environment: %w", err)
	}
	environment, err := parseContainerdProcessEnvironment(output)
	if err != nil {
		return err
	}
	return validateContainerdProcessEnvironment(environment, expectation)
}

func parseContainerdProcessEnvironment(output string) (map[string]string, error) {
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(output))
	if err != nil {
		return nil, fmt.Errorf("decode running containerd environment: %w", err)
	}
	environment := make(map[string]string)
	for _, entry := range strings.Split(string(decoded), "\x00") {
		if entry == "" {
			continue
		}
		key, value, ok := strings.Cut(entry, "=")
		if !ok || key == "" {
			return nil, fmt.Errorf("decode running containerd environment: malformed entry")
		}
		environment[key] = value
	}
	return environment, nil
}

func validateContainerdProcessEnvironment(environment map[string]string, expectation containerdProxyExpectation) error {
	for _, variable := range []string{"ALL_PROXY", "all_proxy"} {
		if strings.TrimSpace(environment[variable]) != "" {
			return fmt.Errorf("running containerd has an active %s that may override the selected registry route; inspect systemctl cat containerd", variable)
		}
	}
	proxyConfig := &httpproxy.Config{
		HTTPProxy:  firstNonEmptyEnvironment(environment, "HTTP_PROXY", "http_proxy"),
		HTTPSProxy: firstNonEmptyEnvironment(environment, "HTTPS_PROXY", "https_proxy"),
		NoProxy:    firstNonEmptyEnvironment(environment, "NO_PROXY", "no_proxy"),
	}
	proxyForURL := proxyConfig.ProxyFunc()
	for _, target := range registryRouteTargets {
		decision := registryDecisionForTarget(expectation.Routing, target.namespace)
		if decision.CanonicalRequired {
			if err := verifyContainerdTargetRoute(proxyForURL, target.name+" canonical", target.canonicalURL, decision.CanonicalRoute, expectation.ProxyURL); err != nil {
				return err
			}
		}
		if decision.MirrorEnabled {
			if err := verifyContainerdTargetRoute(proxyForURL, target.name+" mirror", target.mirrorURL, decision.MirrorRoute, expectation.ProxyURL); err != nil {
				return err
			}
		}
	}
	return nil
}

func firstNonEmptyEnvironment(environment map[string]string, names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(environment[name]); value != "" {
			return value
		}
	}
	return ""
}

func registryDecisionForTarget(routing registryRoutingSelection, namespace string) registryRouteDecision {
	if namespace == "docker.io" {
		return routing.DockerHub
	}
	return routing.Kubernetes
}

func verifyContainerdTargetRoute(
	proxyForURL func(*url.URL) (*url.URL, error),
	targetName string,
	targetURL string,
	expectedRoute registryEgressRoute,
	selectedProxy string,
) error {
	target, err := url.Parse(targetURL)
	if err != nil {
		return fmt.Errorf("verify running containerd route for %s: invalid target", targetName)
	}
	actualProxy, err := proxyForURL(target)
	if err != nil {
		return fmt.Errorf("verify running containerd route for %s: proxy selection failed", targetName)
	}
	switch expectedRoute {
	case registryEgressDirect:
		if actualProxy != nil {
			return fmt.Errorf("running containerd route for %s unexpectedly uses a proxy; inspect systemctl cat containerd and systemctl show containerd --property=Environment", targetName)
		}
	case registryEgressProxy:
		if actualProxy == nil {
			return fmt.Errorf("running containerd route for %s unexpectedly bypasses the selected proxy; inspect systemctl cat containerd and systemctl show containerd --property=Environment", targetName)
		}
		actual, err := normalizeContainerdProxyURL(actualProxy.String())
		if err != nil {
			return fmt.Errorf("running containerd route for %s uses an invalid proxy", targetName)
		}
		expected, err := normalizeContainerdProxyURL(selectedProxy)
		if err != nil || actual != expected {
			return fmt.Errorf("running containerd route for %s uses an unexpected proxy; inspect systemctl cat containerd and systemctl show containerd --property=Environment", targetName)
		}
	default:
		return fmt.Errorf("verify running containerd route for %s: unsupported expected route %q", targetName, expectedRoute)
	}
	return nil
}

func normalizeContainerdProxyURL(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Hostname() == "" || parsed.Port() == "" {
		return "", fmt.Errorf("invalid proxy URL")
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	switch parsed.Scheme {
	case "http", "https", "socks5", "socks5h":
	default:
		return "", fmt.Errorf("invalid proxy URL")
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil || port < 1 || port > 65535 || parsed.Path != "" && parsed.Path != "/" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("invalid proxy URL")
	}
	if parsed.Scheme == "socks5" {
		parsed.Scheme = "socks5h"
	}
	parsed.Host = net.JoinHostPort(strings.ToLower(parsed.Hostname()), parsed.Port())
	parsed.Path = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}
