package deploy

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/casosorg/casos/server"
)

const nodeDeployResolverPath = "/etc/casos-resolv.conf"

const (
	dockerHubHostsPath   = "/etc/containerd/certs.d/docker.io/hosts.toml"
	k8sRegistryHostsPath = "/etc/containerd/certs.d/registry.k8s.io/hosts.toml"
)

type registryMirrorFileRunner interface {
	RunRootContext(ctx context.Context, command string) (string, error)
	RunRootInputSensitiveContext(ctx context.Context, command, input string) (string, error)
}

func (d *NodeDeployer) reconcileContainerdFilesWithAutoFallback(
	ctx context.Context,
	runner containerdLifecycleRunner,
	desired []containerdFileSpec,
	options containerdReconcileOptions,
	fallback func(string) ([]containerdFileSpec, containerdReconcileOptions, error),
) error {
	if ctx == nil {
		ctx = context.Background()
	}
	initialErr := reconcileContainerdFiles(ctx, runner, desired, options)
	if initialErr == nil {
		return nil
	}
	if d == nil || d.config.RegistryMirrorMode != server.RegistryMirrorModeAuto || ctx.Err() != nil {
		return initialErr
	}

	var retrySafeErr *containerdRetrySafeError
	var pullErr *containerdImagePullVerificationError
	if !errors.As(initialErr, &retrySafeErr) || !errors.As(initialErr, &pullErr) {
		return initialErr
	}

	namespace := pullErr.RegistryNamespace()
	decision, ok := containerdRegistryDecisionForNamespace(options.ProxyExpectation.Routing, namespace)
	if !ok || decision.RegistryNamespace != namespace || !decision.CanonicalRequired || decision.MirrorEnabled ||
		!validContainerdEgressRoute(decision.CanonicalRoute) ||
		decision.CanonicalRoute == registryEgressProxy && strings.TrimSpace(options.ProxyExpectation.ProxyURL) == "" {
		return initialErr
	}
	if fallback == nil {
		return joinContainerdFallbackErrors(initialErr, fmt.Errorf("fallback configuration callback is required"))
	}

	fallbackDesired, fallbackOptions, fallbackErr := fallback(namespace)
	if fallbackErr != nil {
		return joinContainerdFallbackErrors(initialErr, fallbackErr)
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return joinContainerdFallbackErrors(initialErr, fmt.Errorf("context is no longer live: %w", ctxErr))
	}
	if err := validateContainerdMirrorFallback(namespace, fallbackOptions); err != nil {
		return joinContainerdFallbackErrors(initialErr, err)
	}
	if err := reconcileContainerdFiles(ctx, runner, fallbackDesired, fallbackOptions); err != nil {
		return joinContainerdFallbackErrors(initialErr, err)
	}
	return nil
}

func containerdRegistryDecisionForNamespace(routing registryRoutingSelection, namespace string) (registryRouteDecision, bool) {
	switch namespace {
	case "docker.io":
		return routing.DockerHub, true
	case "registry.k8s.io":
		return routing.Kubernetes, true
	default:
		return registryRouteDecision{}, false
	}
}

func validateContainerdMirrorFallback(namespace string, options containerdReconcileOptions) error {
	decision, ok := containerdRegistryDecisionForNamespace(options.ProxyExpectation.Routing, namespace)
	if !ok || decision.RegistryNamespace != namespace {
		return fmt.Errorf("invalid containerd mirror fallback routing for namespace %q", namespace)
	}
	if decision.CanonicalRequired || !decision.MirrorEnabled || !validContainerdEgressRoute(decision.MirrorRoute) {
		return fmt.Errorf("invalid containerd mirror fallback routing for namespace %q: a single mirror route is required", namespace)
	}
	if decision.MirrorRoute == registryEgressProxy && strings.TrimSpace(options.ProxyExpectation.ProxyURL) == "" {
		return fmt.Errorf("invalid containerd mirror fallback routing for namespace %q: proxy route has no selected proxy", namespace)
	}
	return nil
}

func validContainerdEgressRoute(route registryEgressRoute) bool {
	return route == registryEgressDirect || route == registryEgressProxy
}

func joinContainerdFallbackErrors(initialErr, fallbackErr error) error {
	return errors.Join(
		fmt.Errorf("initial canonical containerd registry pull failed: %w", initialErr),
		fmt.Errorf("containerd registry mirror fallback failed: %w", fallbackErr),
	)
}

func (d *NodeDeployer) prepareContainerdReconciliation(
	ctx context.Context,
	runner *NodeDeploySSHRunner,
	egress workerEgressSelection,
	configAdoption containerdConfigAdoptionDecision,
	apiserverURL string,
	runtimeVersion containerdRuntimeVersion,
) ([]containerdFileSpec, containerdReconcileOptions, []string, error) {
	var workerHosts []string
	var err error
	if egress.ProxyURL != "" {
		workerHosts, err = discoverContainerdWorkerHosts(ctx, runner)
		if err != nil {
			return nil, containerdReconcileOptions{}, nil, err
		}
	}
	desired, noProxy, err := buildContainerdDesiredFilesForRuntime(
		d.config,
		egress.Routing,
		egress.ProxyURL,
		workerHosts,
		apiserverURL,
		runtimeVersion,
	)
	if err != nil {
		return nil, containerdReconcileOptions{}, nil, fmt.Errorf("render containerd configuration: %w", err)
	}
	return desired, containerdReconcileOptions{
		SandboxImage:   d.config.SandboxImage,
		ConfigAdoption: configAdoption,
		ProxyExpectation: containerdProxyExpectation{
			ProxyURL: egress.ProxyURL,
			Routing:  egress.Routing,
		},
		Warn: func(warning string) {
			d.log("warning", warning, nodeDeployPhaseConfiguring)
		},
	}, noProxy, nil
}

func (d *NodeDeployer) installNodeBinaries(ctx context.Context, runner *NodeDeploySSHRunner, arch, k8sVersion string, wsl bool, apiserverURL string, adoptContainerdConfig bool) error {
	version := k8sVersion
	cniVersion := defaultNodeDeployCNIVersion
	containerdInstalledBefore, err := inspectContainerdPackageInstalled(ctx, runner)
	if err != nil {
		return err
	}
	containerdConfigBefore, err := runner.InspectFileContext(ctx, containerdConfigPath)
	if err != nil {
		return fmt.Errorf("inspect containerd config before package installation: %w", err)
	}

	d.logStep(nodeDeployPhaseInstalling, "Installing node dependencies and containerd")
	if _, err := runner.RunRootContext(ctx, "dpkg -s ca-certificates curl iptables socat conntrack ebtables ethtool kmod containerd >/dev/null 2>&1 || (apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y ca-certificates curl iptables socat conntrack ebtables ethtool kmod containerd)"); err != nil {
		return fmt.Errorf("install packages: %w", err)
	}
	runtimeVersion, err := inspectContainerdRuntimeVersion(ctx, runner)
	if err != nil {
		return err
	}

	d.logStep(nodeDeployPhaseConfiguring, "Preflighting containerd egress routes")
	configAdoption, err := resolveContainerdConfigAdoption(ctx, runner, containerdInstalledBefore, containerdConfigBefore, adoptContainerdConfig)
	if err != nil {
		return err
	}
	policy := buildWorkerEgressPolicy(d.config)
	factsProvider := func() (workerNetworkFacts, error) {
		return discoverWorkerNetworkFacts(ctx, runner, wsl)
	}
	egress, err := d.resolveWorkerEgress(ctx, runner, policy, factsProvider)
	if err != nil {
		return fmt.Errorf("resolve worker containerd egress: %w", err)
	}
	desiredFiles, reconcileOptions, noProxy, err := d.prepareContainerdReconciliation(
		ctx,
		runner,
		egress,
		configAdoption,
		apiserverURL,
		runtimeVersion,
	)
	if err != nil {
		return err
	}
	backupPath, err := backupContainerdConfigForAdoption(ctx, runner, configAdoption)
	if err != nil {
		return err
	}
	if backupPath != "" {
		d.logStep(nodeDeployPhaseConfiguring, "Backed up existing containerd config to "+backupPath)
	}

	if _, err := runner.RunRootContext(ctx, `set -e
install -d /etc/modules-load.d /etc/sysctl.d
printf '%s\n' overlay br_netfilter vxlan > /etc/modules-load.d/casos-kubernetes.conf
modprobe overlay
modprobe br_netfilter
modprobe vxlan
cat > /etc/sysctl.d/99-casos-kubernetes.conf <<'EOF'
net.bridge.bridge-nf-call-iptables = 1
net.bridge.bridge-nf-call-ip6tables = 1
net.ipv4.ip_forward = 1
EOF
sysctl --system >/dev/null
test -e /proc/sys/net/bridge/bridge-nf-call-iptables`); err != nil {
		return fmt.Errorf("configure Kubernetes kernel networking: %w", err)
	}
	if _, err := runner.RunRootContext(ctx, fmt.Sprintf(`set -e
if systemctl is-active --quiet systemd-resolved 2>/dev/null; then
  for i in $(seq 1 30); do
    [ -f /run/systemd/resolve/resolv.conf ] && break
    sleep 1
  done
  test -f /run/systemd/resolve/resolv.conf
  resolver=/run/systemd/resolve/resolv.conf
else
  resolver=/etc/resolv.conf
fi
ln -sfn "$resolver" %[1]s
test -f %[1]s`, nodeDeployResolverPath)); err != nil {
		return fmt.Errorf("configure node resolver: %w", err)
	}

	d.logStep(nodeDeployPhaseConfiguring, "Configuring containerd")
	if egress.ProxyURL != "" {
		d.logStep(nodeDeployPhaseConfiguring, fmt.Sprintf("Containerd egress proxy selected from %s: %s (%d direct destinations)", egress.ProxySource, redactProxyURL(egress.ProxyURL), len(noProxy)))
	} else {
		d.logStep(nodeDeployPhaseConfiguring, "Containerd egress proxy disabled; CasOS-managed proxy files will be removed")
	}
	err = d.reconcileContainerdFilesWithAutoFallback(ctx, runner, desiredFiles, reconcileOptions, func(namespace string) ([]containerdFileSpec, containerdReconcileOptions, error) {
		d.log("warning", fmt.Sprintf("Registry reachability probes for %s succeeded, but the real containerd CRI image pull failed; retrying that namespace once through its built-in mirror", namespace), nodeDeployPhaseConfiguring)

		fallbackPolicy, err := policy.withForcedMirrorNamespace(namespace)
		if err != nil {
			return nil, containerdReconcileOptions{}, err
		}
		fallbackEgress, err := d.resolveWorkerEgress(ctx, runner, fallbackPolicy, factsProvider)
		if err != nil {
			return nil, containerdReconcileOptions{}, fmt.Errorf("resolve mirror fallback egress for %s: %w", namespace, err)
		}
		fallbackDesired, fallbackOptions, fallbackNoProxy, err := d.prepareContainerdReconciliation(
			ctx,
			runner,
			fallbackEgress,
			configAdoption,
			apiserverURL,
			runtimeVersion,
		)
		if err != nil {
			return nil, containerdReconcileOptions{}, err
		}
		if fallbackEgress.ProxyURL != "" {
			d.logStep(nodeDeployPhaseConfiguring, fmt.Sprintf("Containerd mirror fallback for %s selected a proxy from %s: %s (%d direct destinations)", namespace, fallbackEgress.ProxySource, redactProxyURL(fallbackEgress.ProxyURL), len(fallbackNoProxy)))
		} else {
			d.logStep(nodeDeployPhaseConfiguring, fmt.Sprintf("Containerd mirror fallback for %s selected a direct mirror route", namespace))
		}
		return fallbackDesired, fallbackOptions, nil
	})
	if err != nil {
		return fmt.Errorf("reconcile containerd configuration: %w", err)
	}
	d.logStep(nodeDeployPhaseConfiguring, "Reconciled containerd configuration and verified required registry pulls through containerd CRI")

	d.logStep(nodeDeployPhaseInstalling, "Ensuring upstream kubelet, kube-proxy, and CNI plugins")
	installCmd := fmt.Sprintf(`set -e
download() {
  url="$3"
  curl -fsSL --connect-timeout 20 --max-time 600 --retry 2 --retry-delay 5 --retry-connrefused "$@" || { echo "download failed: $url" >&2; exit 22; }
}
needs_kube_binary() {
  path="$1"
  if [ ! -x "$path" ]; then
    return 0
  fi
  "$path" --version 2>/dev/null | grep -Fq "Kubernetes %s" && return 1
  return 0
}
if needs_kube_binary /usr/local/bin/kubelet; then
  download -o /tmp/kubelet https://dl.k8s.io/release/%s/bin/linux/%s/kubelet
  install -o root -g root -m 0755 /tmp/kubelet /usr/local/bin/kubelet
fi
if needs_kube_binary /usr/local/bin/kube-proxy; then
  download -o /tmp/kube-proxy https://dl.k8s.io/release/%s/bin/linux/%s/kube-proxy
  install -o root -g root -m 0755 /tmp/kube-proxy /usr/local/bin/kube-proxy
fi
mkdir -p /opt/cni/bin /etc/cni/net.d
if [ ! -x /opt/cni/bin/bridge ] || [ ! -x /opt/cni/bin/loopback ] || [ ! -x /opt/cni/bin/portmap ]; then
  download -o /tmp/cni-plugins.tgz https://github.com/containernetworking/plugins/releases/download/%s/cni-plugins-linux-%s-%s.tgz
  tar -xzf /tmp/cni-plugins.tgz -C /opt/cni/bin
fi`, version, version, arch, version, arch, cniVersion, arch, cniVersion)
	if _, err := runner.RunRootContext(ctx, installCmd); err != nil {
		return fmt.Errorf("install node binaries: %w", err)
	}
	return nil
}

func (d *NodeDeployer) writeNodeFiles(ctx context.Context, runner *NodeDeploySSHRunner, nodeName, kubeconfig string) error {
	ca, err := extractCertificateAuthority(kubeconfig)
	if err != nil {
		return err
	}
	if err = runner.WriteFileContext(ctx, "/etc/kubernetes/worker.kubeconfig", kubeconfig, "0600"); err != nil {
		return fmt.Errorf("write /etc/kubernetes/worker.kubeconfig: %w", err)
	}
	if err = runner.WriteFileContext(ctx, "/etc/kubernetes/ca.crt", ca, "0644"); err != nil {
		return fmt.Errorf("write /etc/kubernetes/ca.crt: %w", err)
	}
	if err = runner.WriteFileContext(ctx, "/var/lib/kubelet/config.yaml", kubeletConfig(), "0644"); err != nil {
		return fmt.Errorf("write /var/lib/kubelet/config.yaml: %w", err)
	}
	if err = runner.WriteFileContext(ctx, "/etc/systemd/system/kubelet.service", kubeletService(nodeName), "0644"); err != nil {
		return fmt.Errorf("write /etc/systemd/system/kubelet.service: %w", err)
	}
	if err = runner.WriteFileContext(ctx, "/var/lib/kube-proxy/config.yaml", kubeProxyConfig(), "0644"); err != nil {
		return fmt.Errorf("write /var/lib/kube-proxy/config.yaml: %w", err)
	}
	if err = runner.WriteFileContext(ctx, "/etc/systemd/system/kube-proxy.service", kubeProxyService(), "0644"); err != nil {
		return fmt.Errorf("write /etc/systemd/system/kube-proxy.service: %w", err)
	}
	return nil
}
