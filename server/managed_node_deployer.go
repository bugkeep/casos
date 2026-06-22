package server

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/beego/beego/logs"
	"github.com/casosorg/casos/object"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func StartManagedNodeTaskRunner(ctx context.Context, adminCfg *rest.Config, srvCfg Config) error {
	if reclaimed, err := object.FailStaleRunningNodeDeployTasks(
		time.Now().Add(-30*time.Second),
		"managed node task was interrupted and has been reclaimed; please retry the operation",
	); err != nil {
		logs.Warning("managed node reclaim stale tasks: %v", err)
	} else if reclaimed > 0 {
		logs.Info("managed node reclaimed %d stale running task(s)", reclaimed)
	}

	go func() {
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				runPendingManagedNodeTasks(ctx, adminCfg, srvCfg)
			}
		}
	}()
	return nil
}

func runPendingManagedNodeTasks(ctx context.Context, adminCfg *rest.Config, srvCfg Config) {
	tasks, err := object.GetPendingNodeDeployTasks(5)
	if err != nil {
		logs.Warning("managed node pending tasks: %v", err)
		return
	}
	for _, task := range tasks {
		go executeManagedNodeTask(ctx, adminCfg, srvCfg, task.Id)
	}
}

func executeManagedNodeTask(ctx context.Context, adminCfg *rest.Config, srvCfg Config, taskId int64) {
	task, err := object.ClaimNodeDeployTask(taskId)
	if err != nil || task == nil {
		return
	}
	node, err := object.GetManagedNode(task.NodeId)
	if err != nil || node == nil {
		return
	}

	switch task.Type {
	case object.NodeTaskTypeInstall:
		err = deployManagedNode(ctx, adminCfg, srvCfg, node, task)
	case object.NodeTaskTypeRepair:
		err = repairManagedNode(ctx, adminCfg, srvCfg, node, task)
	case object.NodeTaskTypeRemove:
		err = removeManagedNode(ctx, adminCfg, node, task)
	default:
		err = fmt.Errorf("unsupported managed node task type: %s", task.Type)
	}

	if err != nil {
		task.Status = object.NodeTaskStatusFailed
		task.ErrorMsg = err.Error()
		_, _ = object.UpdateNodeDeployTask(task)
		node.State = object.ManagedNodeStateFailed
		node.LastError = err.Error()
		_, _ = object.UpdateManagedNode(node)
		_, _ = object.AddNodeDeployLog(&object.NodeDeployLog{
			TaskId:    task.Id,
			Level:     "error",
			Message:   err.Error(),
			CreatedAt: time.Now(),
		})
		return
	}

	task.Status = object.NodeTaskStatusSucceeded
	task.Progress = 100
	task.Stage = "completed"
	task.FinishedAt = time.Now()
	task.ErrorMsg = ""
	_, _ = object.UpdateNodeDeployTask(task)
}

func deployManagedNode(ctx context.Context, adminCfg *rest.Config, srvCfg Config, node *object.ManagedNode, task *object.NodeDeployTask) error {
	runner, usedPassword, err := openManagedNodeRunner(node)
	if err != nil {
		return err
	}
	defer runner.Close()

	node.State = object.ManagedNodeStateDeploying
	node.LastError = ""
	node.LastDeployAt = time.Now()
	_, _ = object.UpdateManagedNode(node)

	if err := updateTaskStage(task, "connecting", 5, "connected to remote host"); err != nil {
		return err
	}

	preflight, err := ensureRemoteReady(runner, node, task)
	if err != nil {
		return err
	}
	if err := installManagedNodeDependencies(runner, task, srvCfg, preflight); err != nil {
		return err
	}
	if err := writeManagedNodeFiles(runner, node, task, srvCfg, preflight); err != nil {
		return err
	}
	if err := enableManagedNodeServices(runner, task); err != nil {
		return err
	}
	if err := waitForManagedNodeReady(ctx, adminCfg, node, task); err != nil {
		return err
	}
	if err := applyManagedNodeSpec(ctx, adminCfg, node, task); err != nil {
		return err
	}
	if usedPassword {
		if err := adoptManagedNodeSSHKey(node, runner, task); err != nil {
			return err
		}
	}

	node.State = object.ManagedNodeStateReady
	node.KubernetesStatus = "Ready"
	node.LastSeenAt = time.Now()
	node.LastError = ""
	_, _ = object.UpdateManagedNode(node)
	return nil
}

func repairManagedNode(ctx context.Context, adminCfg *rest.Config, srvCfg Config, node *object.ManagedNode, task *object.NodeDeployTask) error {
	node.State = object.ManagedNodeStateRepairing
	_, _ = object.UpdateManagedNode(node)
	return deployManagedNode(ctx, adminCfg, srvCfg, node, task)
}

func removeManagedNode(ctx context.Context, adminCfg *rest.Config, node *object.ManagedNode, task *object.NodeDeployTask) error {
	if err := updateTaskStage(task, "removing", 20, "stopping managed node services and cleaning remote files"); err != nil {
		return err
	}
	if err := cleanupManagedNodeHost(node); err != nil {
		_, _ = object.AddNodeDeployLog(&object.NodeDeployLog{
			TaskId:    task.Id,
			Level:     "warning",
			Message:   "remote cleanup skipped or failed: " + err.Error(),
			CreatedAt: time.Now(),
		})
	}
	if err := updateTaskStage(task, "removing", 70, "removing kubernetes node and local record"); err != nil {
		return err
	}
	if adminCfg != nil {
		client, err := kubernetes.NewForConfig(adminCfg)
		if err == nil {
			_ = client.CoreV1().Nodes().Delete(ctx, node.Name, metav1.DeleteOptions{})
		}
	}
	return object.DeleteManagedNode(node.Id)
}

func ensureRemoteReady(runner SSHRunner, node *object.ManagedNode, task *object.NodeDeployTask) (*ManagedNodePreflightResult, error) {
	if err := updateTaskStage(task, "detecting", 15, "checking remote operating system"); err != nil {
		return nil, err
	}
	result, err := inspectRemoteEnvironment(runner)
	if err != nil {
		return nil, err
	}
	node.OS = result.OS
	node.Version = result.Version
	node.Arch = result.Arch
	if !result.IsRoot {
		return nil, fmt.Errorf("remote user is not root")
	}
	if !result.SupportsArch {
		return nil, fmt.Errorf("unsupported remote architecture: %s", node.Arch)
	}
	if !result.SupportsOS {
		return nil, fmt.Errorf("unsupported remote operating system: %s %s", node.OS, node.Version)
	}
	if message := preflightFailureMessage(result); message != "" {
		return nil, errors.New(message)
	}
	_, _ = object.UpdateManagedNode(node)
	return result, nil
}

func installManagedNodeDependencies(runner SSHRunner, task *object.NodeDeployTask, srvCfg Config, preflight *ManagedNodePreflightResult) error {
	if err := updateTaskStage(task, "installing", 35, "installing containerd and worker dependencies"); err != nil {
		return err
	}
	commands := []string{
		"export DEBIAN_FRONTEND=noninteractive && apt-get update",
		"export DEBIAN_FRONTEND=noninteractive && apt-get install -y containerd iptables curl ca-certificates socat conntrack",
		"mkdir -p /opt/cni/bin /etc/cni/net.d /etc/containerd/certs.d/docker.io /etc/containerd/certs.d/registry.k8s.io /etc/kubernetes /var/lib/kubelet /var/lib/kube-proxy",
	}
	for _, cmd := range commands {
		_, stderr, err := runner.Run(cmd)
		if err != nil {
			return commandError(cmd, stderr, err)
		}
	}

	kubeletURL := "https://dl.k8s.io/v1.36.1/bin/linux/amd64/kubelet"
	kubeProxyURL := "https://dl.k8s.io/v1.36.1/bin/linux/amd64/kube-proxy"
	cniURL := "https://github.com/containernetworking/plugins/releases/download/v1.5.1/cni-plugins-linux-amd64-v1.5.1.tgz"
	downloadProxy := resolveManagedNodeDownloadProxy(srvCfg.Socks5Proxy, preflight)
	commands = managedNodeDownloadCommands(downloadProxy, kubeletURL, kubeProxyURL, cniURL)
	for _, cmd := range commands {
		_, stderr, err := runner.Run(cmd)
		if err != nil {
			return commandError(cmd, stderr, err)
		}
	}
	return nil
}

func writeManagedNodeFiles(runner SSHRunner, node *object.ManagedNode, task *object.NodeDeployTask, srvCfg Config, preflight *ManagedNodePreflightResult) error {
	if err := updateTaskStage(task, "configuring", 65, "writing kubelet, kube-proxy, and containerd configs"); err != nil {
		return err
	}
	wk, err := GenerateWorkerKubeconfig(srvCfg, node.Name, srvCfg.AdvertiseAddress)
	if err != nil {
		return err
	}
	if preflight == nil {
		preflight, err = inspectRemoteEnvironment(runner)
		if err != nil {
			return err
		}
	}
	if preflight.IsWSL && preflight.WindowsHost != "" {
		wk.Kubeconfig = rewriteWorkerKubeconfigServer(wk.Kubeconfig, srvCfg, preflight.WindowsHost)
	}
	if err := runner.WriteFile("/etc/kubernetes/worker.kubeconfig", wk.Kubeconfig, 0o600); err != nil {
		return err
	}
	if err := runner.WriteFile("/etc/kubernetes/ca.crt", decodeWorkerKubeconfigCA(wk.Kubeconfig), 0o644); err != nil {
		return err
	}
	if err := runner.WriteFile("/etc/containerd/config.toml", GenerateContainerdConfig(srvCfg.SandboxImage, srvCfg.Socks5Proxy), 0o644); err != nil {
		return err
	}
	if srvCfg.Socks5Proxy != "" {
		if err := runner.WriteFile("/etc/containerd/certs.d/docker.io/hosts.toml", GenerateDockerHubHostsToml(), 0o644); err != nil {
			return err
		}
		if err := runner.WriteFile("/etc/containerd/certs.d/registry.k8s.io/hosts.toml", GenerateK8sRegistryHostsToml(), 0o644); err != nil {
			return err
		}
	}
	if err := runner.WriteFile("/etc/cni/net.d/10-bridge.conflist", managedNodeBridgeCNIConfig(), 0o644); err != nil {
		return err
	}
	if err := runner.WriteFile("/var/lib/kubelet/config.yaml", managedNodeKubeletConfig(node.Name), 0o644); err != nil {
		return err
	}
	if err := runner.WriteFile("/var/lib/kube-proxy/config.yaml", managedNodeKubeProxyConfig(), 0o644); err != nil {
		return err
	}
	if err := runner.WriteFile("/etc/systemd/system/kubelet.service", managedNodeKubeletService(), 0o644); err != nil {
		return err
	}
	if err := runner.WriteFile("/etc/systemd/system/kube-proxy.service", managedNodeKubeProxyService(), 0o644); err != nil {
		return err
	}
	return nil
}

func enableManagedNodeServices(runner SSHRunner, task *object.NodeDeployTask) error {
	if err := updateTaskStage(task, "starting", 80, "starting worker services"); err != nil {
		return err
	}
	commands := []string{
		"systemctl daemon-reload",
		"systemctl enable --now containerd",
		"systemctl enable --now kubelet",
		"systemctl enable --now kube-proxy",
	}
	for _, cmd := range commands {
		_, stderr, err := runner.Run(cmd)
		if err != nil {
			return commandError(cmd, stderr, err)
		}
	}
	return nil
}

func waitForManagedNodeReady(ctx context.Context, adminCfg *rest.Config, node *object.ManagedNode, task *object.NodeDeployTask) error {
	if adminCfg == nil {
		return fmt.Errorf("admin rest config not ready")
	}
	if err := updateTaskStage(task, "verifying", 90, "waiting for node to become ready"); err != nil {
		return err
	}
	client, err := kubernetes.NewForConfig(adminCfg)
	if err != nil {
		return err
	}
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	timeout := time.NewTimer(2 * time.Minute)
	defer timeout.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout.C:
			return fmt.Errorf("node %s did not become ready within timeout", node.Name)
		case <-ticker.C:
			remoteNode, err := client.CoreV1().Nodes().Get(ctx, node.Name, metav1.GetOptions{})
			if err != nil {
				continue
			}
			node.KubernetesStatus = managedNodeKubernetesStatus(remoteNode)
			node.LastSeenAt = time.Now()
			_, _ = object.UpdateManagedNode(node)
			if node.KubernetesStatus == "Ready" {
				return nil
			}
		}
	}
}

func applyManagedNodeSpec(ctx context.Context, adminCfg *rest.Config, node *object.ManagedNode, task *object.NodeDeployTask) error {
	if err := updateTaskStage(task, "reconciling", 95, "applying managed node labels and scheduling state"); err != nil {
		return err
	}
	client, err := kubernetes.NewForConfig(adminCfg)
	if err != nil {
		return err
	}
	current, err := client.CoreV1().Nodes().Get(ctx, node.Name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	if current.Labels == nil {
		current.Labels = map[string]string{}
	}
	for key, value := range node.GetLabelMap() {
		current.Labels[key] = value
	}
	current.Spec.Unschedulable = node.Unschedulable
	updated, err := client.CoreV1().Nodes().Update(ctx, current, metav1.UpdateOptions{})
	if err != nil {
		return err
	}
	node.KubernetesStatus = managedNodeKubernetesStatus(updated)
	return nil
}

func StartManagedNodeReconciler(ctx context.Context, adminCfg *rest.Config, srvCfg Config) error {
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				reconcileManagedNodes(ctx, adminCfg, srvCfg)
			}
		}
	}()
	return nil
}

func reconcileManagedNodes(ctx context.Context, adminCfg *rest.Config, srvCfg Config) {
	if adminCfg == nil {
		return
	}
	nodes, err := object.GetManagedNodes()
	if err != nil {
		logs.Warning("managed node reconcile list: %v", err)
		return
	}
	client, err := kubernetes.NewForConfig(adminCfg)
	if err != nil {
		logs.Warning("managed node reconcile client: %v", err)
		return
	}
	for _, node := range nodes {
		if node.State == object.ManagedNodeStateRemoved || node.State == object.ManagedNodeStateRemoving {
			continue
		}
		remoteNode, err := client.CoreV1().Nodes().Get(ctx, node.Name, metav1.GetOptions{})
		if err != nil {
			node.KubernetesStatus = "Missing"
			if node.State == object.ManagedNodeStateReady {
				node.State = object.ManagedNodeStateDegraded
				node.LastError = "kubernetes node not found"
			}
			_, _ = object.UpdateManagedNode(node)
			continue
		}
		node.KubernetesStatus = managedNodeKubernetesStatus(remoteNode)
		node.LastSeenAt = time.Now()
		if node.KubernetesStatus == "Ready" {
			node.State = object.ManagedNodeStateReady
			node.LastError = ""
		} else if node.State == object.ManagedNodeStateReady {
			node.State = object.ManagedNodeStateDegraded
			node.LastError = fmt.Sprintf("kubernetes node status is %s", node.KubernetesStatus)
		}
		if node.PrivateKey != "" || node.EncryptedPassword != "" {
			if err := inspectManagedNodeServices(node); err != nil {
				node.State = object.ManagedNodeStateDegraded
				node.LastError = err.Error()
			}
		}
		_, _ = object.UpdateManagedNode(node)

		if node.State != object.ManagedNodeStateDegraded || !hasManagedNodeMaintenanceCredential(node) {
			continue
		}
		hasTask, err := object.HasActiveNodeDeployTask(node.Id)
		if err != nil || hasTask {
			continue
		}
		task := &object.NodeDeployTask{
			NodeId:   node.Id,
			Type:     object.NodeTaskTypeRepair,
			Status:   object.NodeTaskStatusPending,
			Stage:    "queued",
			Progress: 0,
		}
		_, _ = object.AddNodeDeployTask(task)
		_, _ = object.AddNodeDeployLog(&object.NodeDeployLog{
			TaskId:    task.Id,
			Level:     "info",
			Message:   "queued automatic repair after reconcile",
			CreatedAt: time.Now(),
		})
	}
}

func updateTaskStage(task *object.NodeDeployTask, stage string, progress int, message string) error {
	task.Stage = stage
	task.Progress = progress
	_, err := object.UpdateNodeDeployTask(task)
	if err != nil {
		return err
	}
	if message == "" {
		return nil
	}
	_, err = object.AddNodeDeployLog(&object.NodeDeployLog{
		TaskId:    task.Id,
		Level:     "info",
		Message:   message,
		CreatedAt: time.Now(),
	})
	return err
}

func hasManagedNodeMaintenanceCredential(node *object.ManagedNode) bool {
	if node == nil {
		return false
	}
	return strings.TrimSpace(node.PrivateKey) != "" || strings.TrimSpace(node.EncryptedPassword) != ""
}

func commandError(command, stderr string, err error) error {
	if stderr == "" {
		return fmt.Errorf("%s: %w", command, err)
	}
	return fmt.Errorf("%s: %w: %s", command, err, stderr)
}

func managedNodeDownloadCommands(downloadProxy, kubeletURL, kubeProxyURL, cniURL string) []string {
	return []string{
		managedNodeDownloadCommand(downloadProxy, "/usr/local/bin/kubelet", kubeletURL, "test -x /usr/local/bin/kubelet", "chmod 0755 /usr/local/bin/kubelet.tmp-casos && mv -f /usr/local/bin/kubelet.tmp-casos /usr/local/bin/kubelet"),
		managedNodeDownloadCommand(downloadProxy, "/usr/local/bin/kube-proxy", kubeProxyURL, "test -x /usr/local/bin/kube-proxy", "chmod 0755 /usr/local/bin/kube-proxy.tmp-casos && mv -f /usr/local/bin/kube-proxy.tmp-casos /usr/local/bin/kube-proxy"),
		managedNodeDownloadCommand(downloadProxy, "/tmp/cni.tgz", cniURL, "test -x /opt/cni/bin/bridge && test -x /opt/cni/bin/loopback && test -x /opt/cni/bin/host-local && test -x /opt/cni/bin/portmap", "mv -f /tmp/cni.tgz.tmp-casos /tmp/cni.tgz && tar -xzf /tmp/cni.tgz -C /opt/cni/bin"),
	}
}

func managedNodeDownloadCommand(downloadProxy, outputPath, artifactURL, skipCheck, finalize string) string {
	if strings.TrimSpace(skipCheck) != "" {
		prefixed := managedNodeDownloadCommand(downloadProxy, outputPath, artifactURL, "", finalize)
		return fmt.Sprintf("(%s) || (%s)", skipCheck, prefixed)
	}
	tempPath := outputPath + ".tmp-casos"
	base := fmt.Sprintf("rm -f %q && curl -fsSL -o %q %q", tempPath, tempPath, artifactURL)
	if strings.TrimSpace(downloadProxy) != "" {
		base = fmt.Sprintf(
			"rm -f %q && (curl -fsSL --connect-timeout 5 --max-time 15 --proxy %q -o %q %q || curl -fsSL -o %q %q)",
			tempPath,
			downloadProxy,
			tempPath,
			artifactURL,
			tempPath,
			artifactURL,
		)
	}
	if strings.TrimSpace(finalize) == "" {
		return base
	}
	return fmt.Sprintf("%s && %s", base, finalize)
}

func managedNodeBridgeCNIConfig() string {
	return `{
  "cniVersion": "1.0.0",
  "name": "bridge",
  "plugins": [
    {
      "type": "bridge",
      "bridge": "cni0",
      "isGateway": true,
      "ipMasq": true,
      "ipam": {
        "type": "host-local",
        "ranges": [[{"subnet": "10.244.0.0/24"}]],
        "routes": [{"dst": "0.0.0.0/0"}]
      }
    },
    {"type": "portmap", "capabilities": {"portMappings": true}},
    {"type": "loopback"}
  ]
}
`
}

func managedNodeKubeletConfig(nodeName string) string {
	return fmt.Sprintf(`apiVersion: kubelet.config.k8s.io/v1beta1
kind: KubeletConfiguration
nodeName: %s
cgroupDriver: systemd
failSwapOn: false
containerRuntimeEndpoint: unix:///run/containerd/containerd.sock
clusterDNS:
  - 10.43.0.10
clusterDomain: cluster.local
`, nodeName)
}

func managedNodeKubeProxyConfig() string {
	return `apiVersion: kubeproxy.config.k8s.io/v1alpha1
kind: KubeProxyConfiguration
clientConnection:
  kubeconfig: /etc/kubernetes/worker.kubeconfig
mode: iptables
clusterCIDR: 10.42.0.0/16
`
}

func managedNodeKubeletService() string {
	return `[Unit]
Description=Kubernetes Kubelet
After=containerd.service
Requires=containerd.service

[Service]
ExecStart=/usr/local/bin/kubelet \
  --kubeconfig=/etc/kubernetes/worker.kubeconfig \
  --config=/var/lib/kubelet/config.yaml \
  --client-ca-file=/etc/kubernetes/ca.crt \
  --register-node=true \
  --v=2
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
`
}

func managedNodeKubeProxyService() string {
	return `[Unit]
Description=Kubernetes Kube-Proxy
After=network.target

[Service]
ExecStart=/usr/local/bin/kube-proxy \
  --config=/var/lib/kube-proxy/config.yaml \
  --v=2
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
`
}

func managedNodeKubernetesStatus(node *corev1.Node) string {
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady {
			if condition.Status == corev1.ConditionTrue {
				return "Ready"
			}
			return "NotReady"
		}
	}
	return "Unknown"
}

func openManagedNodeRunner(node *object.ManagedNode) (SSHRunner, bool, error) {
	if strings.TrimSpace(node.PrivateKey) != "" {
		runner, err := NewSSHRunnerWithPrivateKey(node.Host, node.Port, node.Username, node.PrivateKey)
		if err == nil {
			return runner, false, nil
		}
	}
	password, err := DecryptManagedNodePassword(node.EncryptedPassword)
	if err != nil {
		return nil, false, err
	}
	runner, err := NewSSHRunner(node.Host, node.Port, node.Username, password)
	if err != nil {
		return nil, false, err
	}
	return runner, true, nil
}

func adoptManagedNodeSSHKey(node *object.ManagedNode, runner SSHRunner, task *object.NodeDeployTask) error {
	if node.PrivateKey != "" && node.PublicKey != "" {
		return nil
	}
	if err := updateTaskStage(task, "hardening", 97, "installing managed SSH key for future maintenance"); err != nil {
		return err
	}
	privateKey, publicKey, err := GenerateManagedNodeSSHKeyPair()
	if err != nil {
		return err
	}
	if err := runner.AppendAuthorizedKey(publicKey); err != nil {
		return err
	}
	node.PrivateKey = privateKey
	node.PublicKey = publicKey
	node.EncryptedPassword = ""
	_, err = object.UpdateManagedNode(node)
	return err
}

func decodeWorkerKubeconfigCA(kubeconfig string) string {
	for _, line := range strings.Split(kubeconfig, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "certificate-authority-data:") {
			continue
		}
		parts := strings.SplitN(trimmed, ":", 2)
		if len(parts) != 2 {
			continue
		}
		data, err := base64.StdEncoding.DecodeString(strings.TrimSpace(parts[1]))
		if err != nil {
			return ""
		}
		return string(data)
	}
	return ""
}

type ManagedNodePreflightResult struct {
	Reachable    bool   `json:"reachable"`
	IsRoot       bool   `json:"isRoot"`
	SupportsOS   bool   `json:"supportsOs"`
	SupportsArch bool   `json:"supportsArch"`
	HasSystemd   bool   `json:"hasSystemd"`
	InitProcess  string `json:"initProcess"`
	SystemdState string `json:"systemdState"`
	IsWSL        bool   `json:"isWsl"`
	WindowsHost  string `json:"windowsHost"`
	OS           string `json:"os"`
	Version      string `json:"version"`
	Arch         string `json:"arch"`
	Message      string `json:"message"`
}

func PreflightManagedNode(host string, port int, username, password string) (*ManagedNodePreflightResult, error) {
	result := &ManagedNodePreflightResult{}
	runner, err := NewSSHRunner(host, port, username, password)
	if err != nil {
		result.Message = "ssh connection failed"
		return result, err
	}
	defer runner.Close()
	result.Reachable = true
	result, err = inspectRemoteEnvironment(runner)
	if err != nil {
		return result, err
	}
	result.Message = preflightMessage(result)
	return result, nil
}

func inspectRemoteEnvironment(runner SSHRunner) (*ManagedNodePreflightResult, error) {
	command := `uid=$(id -u)
if [ -f /etc/os-release ]; then . /etc/os-release; fi
arch=$(uname -m)
has_systemd_cmd=0
pid1=$(ps -p 1 -o comm= 2>/dev/null | tr -d ' ')
systemd_state=
if command -v systemctl >/dev/null 2>&1; then
  has_systemd_cmd=1
  systemd_state=$(systemctl is-system-running 2>/dev/null || true)
fi
is_wsl=0
windows_host=
if grep -qi microsoft /proc/version 2>/dev/null; then
  is_wsl=1
  windows_host=$(ip route 2>/dev/null | awk '/default/ {print $3; exit}')
fi
printf '%s|%s|%s|%s|%s|%s|%s|%s|%s' "$uid" "$ID" "$VERSION_ID" "$arch" "$has_systemd_cmd" "$pid1" "$systemd_state" "$is_wsl" "$windows_host"`
	stdout, stderr, err := runner.Run(command)
	if err != nil {
		return nil, commandError("detect remote environment", stderr, err)
	}
	parts := strings.Split(stdout, "|")
	if len(parts) != 9 {
		return nil, fmt.Errorf("unexpected remote environment response: %s", stdout)
	}
	initProcess := strings.TrimSpace(parts[5])
	systemdState := strings.TrimSpace(parts[6])
	hasSystemd := strings.TrimSpace(parts[4]) == "1" && initProcess == "systemd"
	if systemdState == "" || systemdState == "offline" || systemdState == "unknown" {
		hasSystemd = false
	}
	result := &ManagedNodePreflightResult{
		Reachable:    true,
		IsRoot:       strings.TrimSpace(parts[0]) == "0",
		OS:           strings.TrimSpace(parts[1]),
		Version:      strings.TrimSpace(parts[2]),
		Arch:         normalizeArch(strings.TrimSpace(parts[3])),
		HasSystemd:   hasSystemd,
		InitProcess:  initProcess,
		SystemdState: systemdState,
		IsWSL:        strings.TrimSpace(parts[7]) == "1",
		WindowsHost:  strings.TrimSpace(parts[8]),
	}
	result.SupportsOS = result.OS == "ubuntu" || result.OS == "debian"
	result.SupportsArch = result.Arch == "amd64"
	result.Message = preflightMessage(result)
	return result, nil
}

func preflightFailureMessage(result *ManagedNodePreflightResult) string {
	if result == nil {
		return "unknown preflight result"
	}
	switch {
	case !result.Reachable:
		return "host is not reachable over SSH"
	case !result.IsRoot:
		return "remote account is not root"
	case !result.SupportsOS:
		return fmt.Sprintf("unsupported operating system: %s %s", result.OS, result.Version)
	case !result.SupportsArch:
		return fmt.Sprintf("unsupported architecture: %s", result.Arch)
	case result.IsWSL && !result.HasSystemd:
		return "WSL detected, but systemd is not active. Enable [boot] systemd=true in /etc/wsl.conf and restart WSL."
	case !result.HasSystemd:
		if result.InitProcess != "" && result.InitProcess != "systemd" {
			return fmt.Sprintf("systemd is required on the remote host, but PID 1 is %s", result.InitProcess)
		}
		return "systemd is required on the remote host and must be active"
	case result.IsWSL && result.WindowsHost == "":
		return "WSL detected, but the Windows host gateway IP could not be resolved. Use NAT networking and make sure `ip route` shows a default gateway."
	default:
		return ""
	}
}

func preflightMessage(result *ManagedNodePreflightResult) string {
	if message := preflightFailureMessage(result); message != "" {
		return message
	}
	switch {
	case result == nil:
		return "unknown preflight result"
	case result.IsWSL:
		return fmt.Sprintf("remote host passed preflight checks (WSL mode, Windows host: %s)", result.WindowsHost)
	default:
		return "remote host passed preflight checks"
	}
}

func normalizeArch(arch string) string {
	switch arch {
	case "x86_64":
		return "amd64"
	default:
		return arch
	}
}

func inspectManagedNodeServices(node *object.ManagedNode) error {
	runner, _, err := openManagedNodeRunner(node)
	if err != nil {
		return fmt.Errorf("ssh reconnect failed: %w", err)
	}
	defer runner.Close()
	for _, service := range []string{"containerd", "kubelet", "kube-proxy"} {
		stdout, stderr, err := runner.Run(fmt.Sprintf("systemctl is-active %s", service))
		if err != nil {
			return commandError("systemctl is-active "+service, stderr, err)
		}
		if strings.TrimSpace(stdout) != "active" {
			return fmt.Errorf("%s is not active: %s", service, stdout)
		}
	}
	return nil
}

func rewriteWorkerKubeconfigServer(kubeconfig string, srvCfg Config, host string) string {
	if host == "" {
		return kubeconfig
	}
	target := fmt.Sprintf("https://%s:%d", host, srvCfg.ApiserverPort)
	if srvCfg.AdvertiseAddress != "" {
		kubeconfig = strings.ReplaceAll(
			kubeconfig,
			fmt.Sprintf("https://%s:%d", srvCfg.AdvertiseAddress, srvCfg.ApiserverPort),
			target,
		)
	}
	return strings.ReplaceAll(
		kubeconfig,
		fmt.Sprintf("https://127.0.0.1:%d", srvCfg.ApiserverPort),
		target,
	)
}

func resolveManagedNodeDownloadProxy(proxy string, preflight *ManagedNodePreflightResult) string {
	proxy = strings.TrimSpace(proxy)
	if proxy == "" {
		return ""
	}
	normalized := proxy
	if !strings.Contains(normalized, "://") {
		normalized = "http://" + normalized
	}
	parsed, err := url.Parse(normalized)
	if err != nil || parsed.Host == "" {
		return normalized
	}
	if preflight != nil && preflight.IsWSL && preflight.WindowsHost != "" && isManagedNodeLoopbackHost(parsed.Hostname()) {
		if port := parsed.Port(); port != "" {
			parsed.Host = net.JoinHostPort(preflight.WindowsHost, port)
		} else {
			parsed.Host = preflight.WindowsHost
		}
	}
	return parsed.String()
}

func isManagedNodeLoopbackHost(host string) bool {
	host = strings.TrimSpace(strings.ToLower(host))
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func cleanupManagedNodeHost(node *object.ManagedNode) error {
	runner, _, err := openManagedNodeRunner(node)
	if err != nil {
		return err
	}
	defer runner.Close()
	commands := []string{
		"systemctl disable --now kube-proxy || true",
		"systemctl disable --now kubelet || true",
		"rm -f /etc/systemd/system/kube-proxy.service /etc/systemd/system/kubelet.service",
		"rm -f /var/lib/kube-proxy/config.yaml /var/lib/kubelet/config.yaml /etc/kubernetes/worker.kubeconfig /etc/kubernetes/ca.crt",
		"systemctl daemon-reload || true",
	}
	for _, command := range commands {
		_, _, err := runner.Run(command)
		if err != nil {
			return err
		}
	}
	return nil
}
