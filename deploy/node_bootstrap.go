package deploy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/retry"
)

type nodeBootstrapState struct {
	podCIDR string
	ready   bool
}

type NodeDeployer struct {
	config     Config
	restConfig *rest.Config
	log        NodeDeployLogger
}

const (
	workerProbeAttemptTimeout = 2 * time.Minute
	flannelDaemonSetName      = "kube-flannel-ds"
	workerBootstrapTaintKey   = "casos.io/bootstrap"
)

var nodeCIDRReservationMu sync.Mutex

func NewNodeDeployer(config Config, restConfig *rest.Config, log NodeDeployLogger) *NodeDeployer {
	if log == nil {
		log = func(string, string, string) {}
	}
	return &NodeDeployer{config: config, restConfig: restConfig, log: log}
}

func (d *NodeDeployer) logStep(phase, message string) {
	d.log("info", message, phase)
}

func (d *NodeDeployer) Preflight(ctx context.Context, opts NodeDeployOptions) (*NodeDeployPreflightResult, error) {
	if err := (&opts).validate(); err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	runner, err := newRunnerForMachine(opts.Machine)
	if err != nil {
		return nil, err
	}
	defer runner.Close()
	return RunNodeDeployPreflight(ctx, runner, opts.ApiserverURL)
}

func (d *NodeDeployer) Deploy(ctx context.Context, opts NodeDeployOptions) (*NodeDeployResult, error) {
	if err := (&opts).validate(); err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if d.restConfig == nil {
		return nil, fmt.Errorf("apiserver rest config is required")
	}
	runner, err := newRunnerForMachine(opts.Machine)
	if err != nil {
		return nil, err
	}
	defer runner.Close()

	d.logStep(nodeDeployPhasePreflight, "Starting node preflight")
	preflightResult, err := RunNodeDeployPreflight(ctx, runner, opts.ApiserverURL)
	if err != nil {
		return nil, err
	}

	d.logStep(nodeDeployPhaseConfiguring, "Generating node kubeconfig")
	if d.config.GenerateKubeconfig == nil {
		return nil, fmt.Errorf("node kubeconfig generator is required")
	}
	wk, err := d.config.GenerateKubeconfig(opts.NodeName, opts.ApiserverURL)
	if err != nil {
		return nil, err
	}

	d.logStep(nodeDeployPhaseInstalling, "Querying apiserver version")
	k8sVersion, err := d.apiserverVersion()
	if err != nil {
		return nil, fmt.Errorf("query apiserver version: %w", err)
	}

	if err = d.installNodeBinaries(ctx, runner, preflightResult.Arch, k8sVersion); err != nil {
		return nil, err
	}
	if err = d.writeNodeFiles(ctx, runner, opts.NodeName, wk.Kubeconfig); err != nil {
		return nil, err
	}

	d.logStep(nodeDeployPhaseConfiguring, "Reserving a unique PodCIDR for the worker")
	podCIDR, err := d.ensureNodeCIDR(ctx, opts.NodeName)
	if err != nil {
		return nil, err
	}
	// Keep kubelet's network readiness independent of Flannel startup. The
	// Flannel DaemonSet removes this temporary config after installing its CNI.
	if err = runner.WriteFileContext(ctx, legacyBridgeCNIConfigPath, bridgeCNIConfig(podCIDR), "0644"); err != nil {
		return nil, fmt.Errorf("write bootstrap bridge CNI config: %w", err)
	}
	if _, err = runner.RunRootContext(ctx, "rm -f /etc/cni/net.d/10-flannel.conflist /run/flannel/subnet.env"); err != nil {
		return nil, fmt.Errorf("clean stale Flannel state: %w", err)
	}
	if err = d.startKubelet(ctx, runner); err != nil {
		return nil, err
	}

	d.logStep(nodeDeployPhaseWaiting, "Waiting for node registration")
	bootstrapState, err := d.waitForNodeBootstrapState(ctx, opts.NodeName)
	if err != nil {
		return nil, fmt.Errorf("waiting for node registration: %w", err)
	}
	if err = d.ensureWorkerBootstrapTaint(ctx, opts.NodeName); err != nil {
		return nil, fmt.Errorf("protect worker during bootstrap: %w", err)
	}

	if err = d.startKubeProxy(ctx, runner); err != nil {
		return nil, err
	}

	if !bootstrapState.ready {
		d.logStep(nodeDeployPhaseWaiting, "Waiting for Node Ready")
		if err = d.waitForNodeReady(ctx, opts.NodeName); err != nil {
			return nil, fmt.Errorf("waiting for Node Ready: %w", err)
		}
	} else {
		d.logStep(nodeDeployPhaseWaiting, "Node is already Ready")
	}

	d.logStep(nodeDeployPhaseWaiting, "Waiting for Flannel to become Ready on the worker")
	if err = d.waitForFlannelReady(ctx, opts.NodeName); err != nil {
		return nil, fmt.Errorf("waiting for Flannel readiness: %w", err)
	}
	d.logStep(nodeDeployPhaseConfiguring, "Removing legacy bridge-only CNI config")
	if _, err = runner.RunRootContext(ctx, removeLegacyBridgeCNICommand()); err != nil {
		return nil, fmt.Errorf("remove legacy bridge-only CNI config: %w", err)
	}

	d.logStep(nodeDeployPhaseConfiguring, "Writing CasOS managed SSH key")
	keyPair, err := GenerateNodeDeployKeyPair()
	if err != nil {
		return nil, err
	}
	if err = runner.AppendAuthorizedKeyContext(ctx, keyPair.PublicKey); err != nil {
		return nil, err
	}

	d.logStep(nodeDeployPhaseReady, "Node registered and Ready")
	return &NodeDeployResult{ManagedPrivateKey: keyPair.PrivateKey}, nil
}

func (d *NodeDeployer) ensureNodeCIDR(ctx context.Context, nodeName string) (string, error) {
	if d.restConfig == nil {
		return "", fmt.Errorf("apiserver rest config is required")
	}
	client, err := kubernetes.NewForConfig(d.restConfig)
	if err != nil {
		return "", err
	}

	nodeCIDRReservationMu.Lock()
	defer nodeCIDRReservationMu.Unlock()

	nodes, err := client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return "", fmt.Errorf("list nodes for PodCIDR reservation: %w", err)
	}
	for i := range nodes.Items {
		if nodes.Items[i].Name != nodeName {
			continue
		}
		cidr := nodeCIDRFromSpec(&nodes.Items[i])
		changed := ensureWorkerBootstrapTaint(&nodes.Items[i])
		if cidr == "" {
			cidr, err = allocateNodeCIDR(nodes.Items)
			if err != nil {
				return "", err
			}
			nodes.Items[i].Spec.PodCIDR = cidr
			nodes.Items[i].Spec.PodCIDRs = []string{cidr}
			changed = true
		}
		if changed {
			if _, err = client.CoreV1().Nodes().Update(ctx, &nodes.Items[i], metav1.UpdateOptions{}); err != nil {
				return "", fmt.Errorf("reserve PodCIDR and bootstrap taint for node %s: %w", nodeName, err)
			}
		}
		return cidr, nil
	}

	cidr, err := allocateNodeCIDR(nodes.Items)
	if err != nil {
		return "", err
	}
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   nodeName,
			Labels: map[string]string{corev1.LabelOSStable: "linux"},
		},
		Spec: corev1.NodeSpec{
			PodCIDR:  cidr,
			PodCIDRs: []string{cidr},
			Taints:   []corev1.Taint{{Key: workerBootstrapTaintKey, Value: "true", Effect: corev1.TaintEffectNoSchedule}},
		},
	}
	if _, err = client.CoreV1().Nodes().Create(ctx, node, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		return "", fmt.Errorf("create worker node with PodCIDR: %w", err)
	}
	return cidr, nil
}

func ensureWorkerBootstrapTaint(node *corev1.Node) bool {
	if node == nil {
		return false
	}
	desired := corev1.Taint{Key: workerBootstrapTaintKey, Value: "true", Effect: corev1.TaintEffectNoSchedule}
	changed := false
	found := false
	taints := make([]corev1.Taint, 0, len(node.Spec.Taints)+1)
	for _, taint := range node.Spec.Taints {
		if taint.Key != workerBootstrapTaintKey {
			taints = append(taints, taint)
			continue
		}
		if !found {
			found = true
			taints = append(taints, desired)
			if taint != desired {
				changed = true
			}
		} else {
			changed = true
		}
	}
	if !found {
		taints = append(taints, desired)
		changed = true
	}
	if changed {
		node.Spec.Taints = taints
	}
	return changed
}

func removeWorkerBootstrapTaint(node *corev1.Node) bool {
	if node == nil {
		return false
	}
	taints := make([]corev1.Taint, 0, len(node.Spec.Taints))
	changed := false
	for _, taint := range node.Spec.Taints {
		if taint.Key == workerBootstrapTaintKey {
			changed = true
			continue
		}
		taints = append(taints, taint)
	}
	if changed {
		node.Spec.Taints = taints
	}
	return changed
}

func (d *NodeDeployer) ensureWorkerBootstrapTaint(ctx context.Context, nodeName string) error {
	return d.updateWorkerBootstrapTaint(ctx, nodeName, ensureWorkerBootstrapTaint)
}

func (d *NodeDeployer) removeWorkerBootstrapTaint(ctx context.Context, nodeName string) error {
	return d.updateWorkerBootstrapTaint(ctx, nodeName, removeWorkerBootstrapTaint)
}

func (d *NodeDeployer) updateWorkerBootstrapTaint(ctx context.Context, nodeName string, mutate func(*corev1.Node) bool) error {
	if d.restConfig == nil {
		return fmt.Errorf("apiserver rest config is required")
	}
	client, err := kubernetes.NewForConfig(d.restConfig)
	if err != nil {
		return err
	}
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		node, err := client.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if !mutate(node) {
			return nil
		}
		_, err = client.CoreV1().Nodes().Update(ctx, node, metav1.UpdateOptions{})
		return err
	})
}

func nodeCIDRFromSpec(node *corev1.Node) string {
	if node == nil {
		return ""
	}
	if len(node.Spec.PodCIDRs) > 0 {
		return node.Spec.PodCIDRs[0]
	}
	return node.Spec.PodCIDR
}

func allocateNodeCIDR(nodes []corev1.Node) (string, error) {
	used := make(map[string]struct{}, len(nodes))
	for i := range nodes {
		if cidr := nodeCIDRFromSpec(&nodes[i]); cidr != "" {
			_, network, err := net.ParseCIDR(cidr)
			if err != nil {
				return "", fmt.Errorf("parse PodCIDR %q for node %s: %w", cidr, nodes[i].Name, err)
			}
			used[network.String()] = struct{}{}
		}
	}

	for subnet := 0; subnet < 256; subnet++ {
		candidate := fmt.Sprintf("10.244.%d.0/24", subnet)
		if _, exists := used[candidate]; !exists {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("no available PodCIDR remains in 10.244.0.0/16")
}

func (d *NodeDeployer) waitForFlannelReady(ctx context.Context, nodeName string) error {
	if d.restConfig == nil {
		return fmt.Errorf("apiserver rest config is required")
	}
	client, err := kubernetes.NewForConfig(d.restConfig)
	if err != nil {
		return err
	}
	deadlineTimer, deadline := deploymentWaitDeadline(ctx)
	ticker := time.NewTicker(2 * time.Second)
	defer deadlineTimer.Stop()
	defer ticker.Stop()
	lastReason := "Flannel Pod has not been created"
	var lastPod *corev1.Pod
	for {
		select {
		case <-ctx.Done():
			if lastPod != nil {
				lastReason = flannelPodFailureReason(lastReason, client, lastPod)
			}
			return fmt.Errorf("%s: %w", lastReason, ctx.Err())
		case <-deadline:
			if lastPod != nil {
				lastReason = flannelPodFailureReason(lastReason, client, lastPod)
			}
			return fmt.Errorf("timed out waiting for Flannel to become Ready on worker %s: %s", nodeName, lastReason)
		case <-ticker.C:
			pods, err := client.CoreV1().Pods("kube-flannel").List(ctx, metav1.ListOptions{
				LabelSelector: "k8s-app=flannel",
			})
			if err != nil {
				return err
			}
			matched := false
			for i := range pods.Items {
				pod := &pods.Items[i]
				if pod.Spec.NodeName != nodeName {
					continue
				}
				matched = true
				if flannelPodReady(pod) {
					return nil
				}
				lastPod = pod.DeepCopy()
				lastReason = flannelPodReadinessReason(pod)
			}
			if !matched {
				lastReason = flannelDaemonSetReadinessReason(ctx, client, nodeName)
			}
		}
	}
}

func flannelPodFailureReason(reason string, client kubernetes.Interface, pod *corev1.Pod) string {
	if !strings.Contains(reason, "CrashLoopBackOff") && !strings.Contains(reason, "terminated") {
		return reason
	}
	tailLines := int64(40)
	logCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stream, err := client.CoreV1().Pods("kube-flannel").GetLogs(pod.Name, &corev1.PodLogOptions{
		Container: "kube-flannel",
		Previous:  true,
		TailLines: &tailLines,
	}).Stream(logCtx)
	if err != nil {
		return fmt.Sprintf("%s (unable to read Flannel logs: %v)", reason, err)
	}
	defer stream.Close()
	data, err := io.ReadAll(stream)
	if err != nil {
		return fmt.Sprintf("%s (unable to read Flannel logs: %v)", reason, err)
	}
	logs := strings.TrimSpace(string(data))
	if logs == "" {
		return reason
	}
	logs = strings.ReplaceAll(logs, "\r\n", " | ")
	logs = strings.ReplaceAll(logs, "\n", " | ")
	if len(logs) > 2000 {
		logs = logs[len(logs)-2000:]
	}
	return fmt.Sprintf("%s: logs: %s", reason, logs)
}

func flannelDaemonSetReadinessReason(ctx context.Context, client kubernetes.Interface, nodeName string) string {
	daemonSet, err := client.AppsV1().DaemonSets("kube-flannel").Get(ctx, flannelDaemonSetName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return "Flannel DaemonSet has not been created"
	}
	if err != nil {
		return "unable to inspect Flannel DaemonSet: " + err.Error()
	}
	return fmt.Sprintf(
		"Flannel Pod has not been scheduled on %s (desired=%d current=%d ready=%d available=%d updated=%d)",
		nodeName,
		daemonSet.Status.DesiredNumberScheduled,
		daemonSet.Status.CurrentNumberScheduled,
		daemonSet.Status.NumberReady,
		daemonSet.Status.NumberAvailable,
		daemonSet.Status.UpdatedNumberScheduled,
	)
}

func flannelPodReadinessReason(pod *corev1.Pod) string {
	if pod == nil {
		return "Flannel Pod is missing"
	}
	for _, status := range append(pod.Status.InitContainerStatuses, pod.Status.ContainerStatuses...) {
		if status.State.Waiting != nil {
			if status.LastTerminationState.Terminated != nil && status.LastTerminationState.Terminated.ExitCode != 0 {
				terminated := status.LastTerminationState.Terminated
				return fmt.Sprintf("Flannel container %s is %s after termination (%s, exit code %d): %s", status.Name, status.State.Waiting.Reason, terminated.Reason, terminated.ExitCode, terminated.Message)
			}
			reason := status.State.Waiting.Reason
			if reason == "" {
				reason = "waiting"
			}
			if status.State.Waiting.Message != "" {
				return fmt.Sprintf("Flannel container %s is %s: %s", status.Name, reason, status.State.Waiting.Message)
			}
			return fmt.Sprintf("Flannel container %s is %s", status.Name, reason)
		}
		if status.State.Terminated != nil {
			if status.State.Terminated.ExitCode == 0 {
				continue
			}
			return fmt.Sprintf("Flannel container %s terminated with %s (exit code %d): %s", status.Name, status.State.Terminated.Reason, status.State.Terminated.ExitCode, status.State.Terminated.Message)
		}
	}
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady && condition.Status != corev1.ConditionTrue && condition.Message != "" {
			return "Flannel Pod is not Ready: " + condition.Message
		}
	}
	if pod.Status.Reason != "" || pod.Status.Message != "" {
		return fmt.Sprintf("Flannel Pod is %s: %s", pod.Status.Reason, pod.Status.Message)
	}
	return "Flannel Pod is not Ready"
}

func newRunnerForMachine(machine NodeDeployMachine) (*NodeDeploySSHRunner, error) {
	return NewNodeDeploySSHRunner(NodeDeploySSHConfig{
		Host:       machine.Host,
		Port:       machine.Port,
		Username:   machine.Username,
		Password:   machine.Password,
		PrivateKey: machine.PrivateKey,
	})
}

func (d *NodeDeployer) waitForNodeBootstrapState(ctx context.Context, nodeName string) (*nodeBootstrapState, error) {
	if d.restConfig == nil {
		return nil, fmt.Errorf("apiserver rest config is required")
	}
	client, err := kubernetes.NewForConfig(d.restConfig)
	if err != nil {
		return nil, err
	}
	deadlineTimer, deadline := deploymentWaitDeadline(ctx)
	ticker := time.NewTicker(3 * time.Second)
	defer deadlineTimer.Stop()
	defer ticker.Stop()
	lastState := &nodeBootstrapState{}
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-deadline:
			if lastState.ready {
				return nil, fmt.Errorf("timed out waiting for PodCIDR assignment after node became Ready")
			}
			return nil, fmt.Errorf("timed out waiting for node registration")
		case <-ticker.C:
			node, err := client.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
			if err != nil {
				if !apierrors.IsNotFound(err) {
					return nil, err
				}
				continue
			}
			state := &nodeBootstrapState{
				podCIDR: node.Spec.PodCIDR,
				ready:   isNodeReady(node),
			}
			lastState = state
			if state.podCIDR != "" {
				return state, nil
			}
			if state.ready {
				d.logStep(nodeDeployPhaseWaiting, "Node is Ready; waiting for PodCIDR assignment")
			}
		}
	}
}

func (d *NodeDeployer) waitForNodeReady(ctx context.Context, nodeName string) error {
	if d.restConfig == nil {
		return fmt.Errorf("apiserver rest config is required")
	}
	client, err := kubernetes.NewForConfig(d.restConfig)
	if err != nil {
		return err
	}
	deadlineTimer, deadline := deploymentWaitDeadline(ctx)
	ticker := time.NewTicker(3 * time.Second)
	defer deadlineTimer.Stop()
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline:
			return fmt.Errorf("timed out waiting for Node Ready")
		case <-ticker.C:
			node, err := client.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
			if err != nil {
				if !apierrors.IsNotFound(err) {
					return err
				}
				continue
			}
			if isNodeReady(node) {
				return nil
			}
		}
	}
}

// WaitForOperational verifies the platform prerequisites that make a worker
// safe for application scheduling. Node Ready alone does not prove that CNI,
// DNS, or the default storage path is usable.
func (d *NodeDeployer) WaitForOperational(ctx context.Context, nodeName string) error {
	if d.restConfig == nil {
		return fmt.Errorf("apiserver rest config is required")
	}
	client, err := kubernetes.NewForConfig(d.restConfig)
	if err != nil {
		return err
	}
	deadlineTimer, deadline := deploymentWaitDeadline(ctx)
	ticker := time.NewTicker(3 * time.Second)
	defer deadlineTimer.Stop()
	defer ticker.Stop()
	var lastReason string
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline:
			if lastReason == "" {
				lastReason = "platform prerequisites are not ready"
			}
			return fmt.Errorf("timed out waiting for worker operational readiness: %s", lastReason)
		case <-ticker.C:
			reason, ready, err := workerOperationalState(ctx, client, nodeName, d.config.StorageProbeImage)
			if err != nil {
				return err
			}
			if ready {
				if err := d.removeWorkerBootstrapTaint(ctx, nodeName); err != nil {
					return fmt.Errorf("remove worker bootstrap taint: %w", err)
				}
				return nil
			}
			lastReason = reason
		}
	}
}

func workerOperationalState(ctx context.Context, client kubernetes.Interface, nodeName, storageProbeImage string) (string, bool, error) {
	node, err := client.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return "node is not registered", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if !isNodeReady(node) {
		return "node is not Ready", false, nil
	}
	if node.Spec.PodCIDR == "" {
		return "node has no PodCIDR", false, nil
	}

	flannelPods, err := client.CoreV1().Pods("kube-flannel").List(ctx, metav1.ListOptions{
		LabelSelector: "k8s-app=flannel",
		FieldSelector: "spec.nodeName=" + nodeName,
	})
	if err != nil {
		return "", false, err
	}
	if len(flannelPods.Items) == 0 || !isPodReady(flannelPods.Items[0]) {
		return "Flannel is not Ready on the worker", false, nil
	}

	dns, err := client.AppsV1().Deployments("kube-system").Get(ctx, "coredns", metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return "CoreDNS Deployment is missing", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if dns.Status.AvailableReplicas < 1 {
		return coreDNSReadinessReason(ctx, client, dns), false, nil
	}

	if _, err := client.StorageV1().StorageClasses().Get(ctx, "local-path", metav1.GetOptions{}); apierrors.IsNotFound(err) {
		return "default local-path StorageClass is missing", false, nil
	} else if err != nil {
		return "", false, err
	}
	hostname := nodeName
	if node.Labels["kubernetes.io/hostname"] != "" {
		hostname = node.Labels["kubernetes.io/hostname"]
	}
	probeCtx, cancel := context.WithTimeout(ctx, workerProbeAttemptTimeout)
	err = waitForStorageProbe(probeCtx, client, nodeName, hostname, storageProbeImage)
	cancel()
	if err != nil {
		return "storage probe: " + err.Error(), false, nil
	}
	probeCtx, cancel = context.WithTimeout(ctx, workerProbeAttemptTimeout)
	err = waitForSchedulerProbe(probeCtx, client, nodeName, hostname, storageProbeImage)
	cancel()
	if err != nil {
		return "scheduler probe: " + err.Error(), false, nil
	}
	return "", true, nil
}

func coreDNSReadinessReason(ctx context.Context, client kubernetes.Interface, deployment *appsv1.Deployment) string {
	pods, err := client.CoreV1().Pods("kube-system").List(ctx, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=coredns,app.kubernetes.io/managed-by=casos",
	})
	if err != nil {
		return "CoreDNS is not Available (unable to inspect Pods: " + err.Error() + ")"
	}
	for _, pod := range pods.Items {
		for _, status := range append(pod.Status.InitContainerStatuses, pod.Status.ContainerStatuses...) {
			if status.State.Waiting != nil {
				reason := status.State.Waiting.Reason
				if reason == "" {
					reason = "waiting"
				}
				if status.State.Waiting.Message != "" {
					return coreDNSPodFailureReason(fmt.Sprintf("CoreDNS Pod %s container %s is %s: %s", pod.Name, status.Name, reason, status.State.Waiting.Message), client, pod)
				}
				return coreDNSPodFailureReason(fmt.Sprintf("CoreDNS Pod %s container %s is %s", pod.Name, status.Name, reason), client, pod)
			}
			if status.State.Terminated != nil && status.State.Terminated.ExitCode != 0 {
				terminated := status.State.Terminated
				return coreDNSPodFailureReason(fmt.Sprintf("CoreDNS Pod %s container %s terminated with %s (exit code %d): %s", pod.Name, status.Name, terminated.Reason, terminated.ExitCode, terminated.Message), client, pod)
			}
		}
		if pod.Status.Phase != corev1.PodRunning {
			return fmt.Sprintf("CoreDNS Pod %s is %s: %s", pod.Name, pod.Status.Phase, pod.Status.Message)
		}
	}
	return fmt.Sprintf(
		"CoreDNS is not Available (desired=%d ready=%d available=%d updated=%d)",
		deployment.Status.Replicas,
		deployment.Status.ReadyReplicas,
		deployment.Status.AvailableReplicas,
		deployment.Status.UpdatedReplicas,
	)
}

func coreDNSPodFailureReason(reason string, client kubernetes.Interface, pod corev1.Pod) string {
	if !strings.Contains(reason, "CrashLoopBackOff") && !strings.Contains(reason, "terminated") {
		return reason
	}
	tailLines := int64(40)
	logCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stream, err := client.CoreV1().Pods("kube-system").GetLogs(pod.Name, &corev1.PodLogOptions{
		Container: "coredns",
		Previous:  true,
		TailLines: &tailLines,
	}).Stream(logCtx)
	if err != nil {
		return fmt.Sprintf("%s (unable to read CoreDNS logs: %v)", reason, err)
	}
	defer stream.Close()
	data, err := io.ReadAll(stream)
	if err != nil {
		return fmt.Sprintf("%s (unable to read CoreDNS logs: %v)", reason, err)
	}
	logs := strings.TrimSpace(string(data))
	if logs == "" {
		return reason
	}
	logs = strings.ReplaceAll(logs, "\r\n", " | ")
	logs = strings.ReplaceAll(logs, "\n", " | ")
	if len(logs) > 2000 {
		logs = logs[len(logs)-2000:]
	}
	return fmt.Sprintf("%s: logs: %s", reason, logs)
}

func isPodReady(pod corev1.Pod) bool {
	if pod.Status.Phase != corev1.PodRunning {
		return false
	}
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}

func storageProbeName(nodeName string) string {
	digest := sha256.Sum256([]byte(nodeName))
	return "casos-storage-" + hex.EncodeToString(digest[:])[:16]
}

func waitForStorageProbe(ctx context.Context, client kubernetes.Interface, nodeName, hostname, image string) error {
	if image == "" {
		image = "docker.io/library/busybox:1.37.0"
	}
	const namespace = "kube-system"
	name := storageProbeName(nodeName)
	cleanup := func() {
		_ = client.CoreV1().Pods(namespace).Delete(context.Background(), name, metav1.DeleteOptions{})
		_ = client.CoreV1().PersistentVolumeClaims(namespace).Delete(context.Background(), name, metav1.DeleteOptions{})
	}
	cleanup()
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, Labels: map[string]string{"casos.io/probe": "worker-storage"}},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			StorageClassName: stringPtr("local-path"),
			Resources: corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{
				corev1.ResourceStorage: resource.MustParse("1Mi"),
			}},
		},
	}
	if _, err := client.CoreV1().PersistentVolumeClaims(namespace).Create(ctx, pvc, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("create storage probe PVC: %w", err)
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, Labels: map[string]string{"casos.io/probe": "worker-storage"}},
		Spec: corev1.PodSpec{
			NodeSelector: map[string]string{"kubernetes.io/hostname": hostname},
			Tolerations: []corev1.Toleration{{
				Key:      "casos.io/bootstrap",
				Operator: corev1.TolerationOpExists,
				Effect:   corev1.TaintEffectNoSchedule,
			}},
			RestartPolicy: corev1.RestartPolicyNever,
			Containers: []corev1.Container{{
				Name: "storage-probe", Image: image, ImagePullPolicy: corev1.PullIfNotPresent,
				Command:      []string{"sh", "-c", "echo casos > /data/probe && test \"$(cat /data/probe)\" = casos"},
				VolumeMounts: []corev1.VolumeMount{{Name: "data", MountPath: "/data"}},
			}},
			Volumes: []corev1.Volume{{Name: "data", VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: name}}}},
		},
	}
	if _, err := client.CoreV1().Pods(namespace).Create(ctx, pod, metav1.CreateOptions{}); err != nil {
		cleanup()
		return fmt.Errorf("create storage probe Pod: %w", err)
	}
	defer cleanup()
	deadlineTimer, deadline := deploymentWaitDeadline(ctx)
	ticker := time.NewTicker(2 * time.Second)
	defer deadlineTimer.Stop()
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline:
			return fmt.Errorf("timed out waiting for storage probe")
		case <-ticker.C:
			current, err := client.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
			if apierrors.IsNotFound(err) {
				continue
			}
			if err != nil {
				return fmt.Errorf("get storage probe Pod: %w", err)
			}
			switch current.Status.Phase {
			case corev1.PodSucceeded:
				return nil
			case corev1.PodFailed:
				return fmt.Errorf("storage probe Pod failed")
			}
		}
	}
}

func schedulerProbeName(nodeName string) string {
	digest := sha256.Sum256([]byte(nodeName))
	return "casos-scheduler-" + hex.EncodeToString(digest[:])[:16]
}

func serviceProbeName(nodeName string) string {
	digest := sha256.Sum256([]byte(nodeName))
	return "casos-service-" + hex.EncodeToString(digest[:])[:16]
}

func workerProbeImage(image string) string {
	if image == "" {
		return "docker.io/library/busybox:1.37.0"
	}
	return image
}

type serviceProbePlacement struct {
	serverNodeName string
	serverHostname string
	clientHostname string
}

func selectServiceProbePlacement(nodes []corev1.Node, targetNodeName string) (serviceProbePlacement, error) {
	var target *corev1.Node
	for i := range nodes {
		if nodes[i].Name == targetNodeName {
			target = &nodes[i]
			break
		}
	}
	if target == nil {
		return serviceProbePlacement{}, fmt.Errorf("target worker %s is not registered", targetNodeName)
	}
	placement := serviceProbePlacement{
		serverNodeName: target.Name,
		serverHostname: nodeHostname(target),
		clientHostname: nodeHostname(target),
	}
	for i := range nodes {
		node := &nodes[i]
		if node.Name == targetNodeName || isControlPlaneNode(node) || !isNodeReady(node) || nodeCIDRFromSpec(node) == "" {
			continue
		}
		placement.serverNodeName = node.Name
		placement.serverHostname = nodeHostname(node)
		break
	}
	return placement, nil
}

func nodeHostname(node *corev1.Node) string {
	if node == nil {
		return ""
	}
	if hostname := node.Labels[corev1.LabelHostname]; hostname != "" {
		return hostname
	}
	return node.Name
}

func isControlPlaneNode(node *corev1.Node) bool {
	if node == nil {
		return false
	}
	_, controlPlane := node.Labels["node-role.kubernetes.io/control-plane"]
	_, master := node.Labels["node-role.kubernetes.io/master"]
	return controlPlane || master
}

func workerBootstrapProbeTolerations() []corev1.Toleration {
	return []corev1.Toleration{{
		Key:      "casos.io/bootstrap",
		Operator: corev1.TolerationOpExists,
		Effect:   corev1.TaintEffectNoSchedule,
	}}
}

func waitForSchedulerProbe(ctx context.Context, client kubernetes.Interface, nodeName, hostname, image string) error {
	const namespace = "kube-system"
	name := schedulerProbeName(nodeName)
	cleanup := func() {
		_ = client.CoreV1().Pods(namespace).Delete(context.Background(), name, metav1.DeleteOptions{})
	}
	cleanup()
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, Labels: map[string]string{"casos.io/probe": "scheduler-placement"}},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			NodeSelector:  map[string]string{"kubernetes.io/hostname": hostname},
			Tolerations: []corev1.Toleration{{
				Key:      "casos.io/bootstrap",
				Operator: corev1.TolerationOpExists,
				Effect:   corev1.TaintEffectNoSchedule,
			}},
			Containers: []corev1.Container{{
				Name: "scheduler-probe", Image: workerProbeImage(image), ImagePullPolicy: corev1.PullIfNotPresent,
				Command: []string{"sh", "-c", "echo casos-scheduler"},
			}},
		},
	}
	if _, err := client.CoreV1().Pods(namespace).Create(ctx, pod, metav1.CreateOptions{}); err != nil {
		cleanup()
		return fmt.Errorf("create scheduler probe Pod: %w", err)
	}
	defer cleanup()
	deadlineTimer, deadline := deploymentWaitDeadline(ctx)
	ticker := time.NewTicker(2 * time.Second)
	defer deadlineTimer.Stop()
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline:
			return fmt.Errorf("timed out waiting for scheduler probe")
		case <-ticker.C:
			current, err := client.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
			if apierrors.IsNotFound(err) {
				continue
			}
			if err != nil {
				return fmt.Errorf("get scheduler probe Pod: %w", err)
			}
			if current.Spec.NodeName != "" && current.Spec.NodeName != nodeName {
				return fmt.Errorf("scheduler placed probe on %s instead of %s", current.Spec.NodeName, nodeName)
			}
			switch current.Status.Phase {
			case corev1.PodSucceeded:
				if current.Spec.NodeName != nodeName {
					return fmt.Errorf("scheduler probe succeeded without binding to %s", nodeName)
				}
				return nil
			case corev1.PodFailed:
				return fmt.Errorf("scheduler probe Pod failed")
			}
		}
	}
}

func waitForServiceProbe(ctx context.Context, client kubernetes.Interface, nodeName, image string) error {
	const namespace = "kube-system"
	name := serviceProbeName(nodeName)
	clientName := name + "-client"
	cleanupCtx, cancelCleanup := context.WithTimeout(ctx, 15*time.Second)
	defer cancelCleanup()
	if err := deleteServiceProbeResources(cleanupCtx, client, namespace, name, clientName); err != nil {
		return err
	}
	if err := waitForServiceProbeResourcesDeleted(cleanupCtx, client, namespace, name, clientName); err != nil {
		return err
	}
	nodes, err := client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("list nodes for service probe: %w", err)
	}
	placement, err := selectServiceProbePlacement(nodes.Items, nodeName)
	if err != nil {
		return err
	}
	labels := map[string]string{"casos.io/probe": "service-routing", "casos.io/probe-name": name}
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, Labels: labels},
		Spec:       corev1.ServiceSpec{Selector: labels, Ports: []corev1.ServicePort{{Name: "http", Port: 80, TargetPort: intstr.FromInt(8080)}}},
	}
	createdService, err := client.CoreV1().Services(namespace).Create(ctx, service, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("create service probe Service: %w", err)
	}
	defer func() { _ = deleteServiceProbeResources(context.Background(), client, namespace, name, clientName) }()
	serverPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, Labels: labels},
		Spec: corev1.PodSpec{
			NodeSelector:  map[string]string{corev1.LabelHostname: placement.serverHostname},
			Tolerations:   workerBootstrapProbeTolerations(),
			RestartPolicy: corev1.RestartPolicyAlways,
			Containers: []corev1.Container{{
				Name: "service-server", Image: workerProbeImage(image), ImagePullPolicy: corev1.PullIfNotPresent,
				Command:        []string{"sh", "-c", "mkdir -p /tmp/www && echo casos-service > /tmp/www/index.html && httpd -f -p 8080 -h /tmp/www"},
				ReadinessProbe: &corev1.Probe{ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: "/", Port: intstr.FromInt(8080), Scheme: corev1.URISchemeHTTP}}, InitialDelaySeconds: 1, PeriodSeconds: 2, TimeoutSeconds: 2, FailureThreshold: 5},
			}},
		},
	}
	if _, err := client.CoreV1().Pods(namespace).Create(ctx, serverPod, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("create service probe server Pod: %w", err)
	}
	serverPod, err = waitForServiceProbePodReady(ctx, client, namespace, name)
	if err != nil {
		return err
	}
	if serverPod.Spec.NodeName != placement.serverNodeName {
		return fmt.Errorf("service probe server scheduled on %s instead of %s", serverPod.Spec.NodeName, placement.serverNodeName)
	}
	if serverPod.Status.PodIP == "" {
		return fmt.Errorf("service probe server has no PodIP")
	}
	clusterIP, err := waitForServiceProbeClusterIP(ctx, client, namespace, createdService.Name)
	if err != nil {
		return err
	}
	clientPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: clientName, Namespace: namespace, Labels: map[string]string{"casos.io/probe": "service-routing-client"}},
		Spec: corev1.PodSpec{
			NodeSelector:  map[string]string{corev1.LabelHostname: placement.clientHostname},
			Tolerations:   workerBootstrapProbeTolerations(),
			RestartPolicy: corev1.RestartPolicyNever,
			Containers: []corev1.Container{{
				Name: "service-client", Image: workerProbeImage(image), ImagePullPolicy: corev1.PullIfNotPresent,
				Command: []string{"sh", "-c", fmt.Sprintf("i=0; while [ $i -lt 30 ]; do wget -qO- http://%s:8080/ | grep -q casos-service && wget -qO- http://%s:80/ | grep -q casos-service && exit 0; i=$((i+1)); sleep 1; done; exit 1", serverPod.Status.PodIP, clusterIP)},
			}},
		},
	}
	if _, err := client.CoreV1().Pods(namespace).Create(ctx, clientPod, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("create service probe client Pod: %w", err)
	}
	clientPod, err = waitForServiceProbePodSucceeded(ctx, client, namespace, clientName)
	if err != nil {
		return err
	}
	if clientPod.Spec.NodeName != nodeName {
		return fmt.Errorf("service probe client scheduled on %s instead of %s", clientPod.Spec.NodeName, nodeName)
	}
	return nil
}

func waitForServiceProbeClusterIP(ctx context.Context, client kubernetes.Interface, namespace, name string) (string, error) {
	deadlineTimer, deadline := deploymentWaitDeadline(ctx)
	ticker := time.NewTicker(2 * time.Second)
	defer deadlineTimer.Stop()
	defer ticker.Stop()
	for {
		current, err := client.CoreV1().Services(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			return "", fmt.Errorf("get service probe Service: %w", err)
		}
		if err == nil && current.Spec.ClusterIP != "" && current.Spec.ClusterIP != corev1.ClusterIPNone {
			return current.Spec.ClusterIP, nil
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-deadline:
			return "", fmt.Errorf("timed out waiting for service probe ClusterIP")
		case <-ticker.C:
		}
	}
}

func waitForServiceProbePodReady(ctx context.Context, client kubernetes.Interface, namespace, name string) (*corev1.Pod, error) {
	deadlineTimer, deadline := deploymentWaitDeadline(ctx)
	ticker := time.NewTicker(2 * time.Second)
	defer deadlineTimer.Stop()
	defer ticker.Stop()
	for {
		pod, err := client.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			// The kubelet may not have created the Pod status yet.
		} else if err != nil {
			return nil, fmt.Errorf("get service probe server Pod: %w", err)
		} else if pod.Status.Phase == corev1.PodFailed {
			return nil, fmt.Errorf("service probe server Pod failed")
		} else if isPodReady(*pod) {
			return pod, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-deadline:
			return nil, fmt.Errorf("timed out waiting for service probe server Pod")
		case <-ticker.C:
		}
	}
}

func waitForServiceProbePodSucceeded(ctx context.Context, client kubernetes.Interface, namespace, name string) (*corev1.Pod, error) {
	deadlineTimer, deadline := deploymentWaitDeadline(ctx)
	ticker := time.NewTicker(2 * time.Second)
	defer deadlineTimer.Stop()
	defer ticker.Stop()
	for {
		pod, err := client.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			// The kubelet may not have created the Pod status yet.
		} else if err != nil {
			return nil, fmt.Errorf("get service probe client Pod: %w", err)
		} else {
			switch pod.Status.Phase {
			case corev1.PodSucceeded:
				return pod, nil
			case corev1.PodFailed:
				return nil, fmt.Errorf("service probe client Pod failed")
			}
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-deadline:
			return nil, fmt.Errorf("timed out waiting for service probe client Pod")
		case <-ticker.C:
		}
	}
}

func deleteServiceProbeResources(ctx context.Context, client kubernetes.Interface, namespace, serviceName, clientName string) error {
	if err := client.CoreV1().Pods(namespace).Delete(ctx, clientName, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete service probe client Pod: %w", err)
	}
	if err := client.CoreV1().Pods(namespace).Delete(ctx, serviceName, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete service probe server Pod: %w", err)
	}
	if err := client.CoreV1().Services(namespace).Delete(ctx, serviceName, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete service probe Service: %w", err)
	}
	return nil
}

func waitForServiceProbeResourcesDeleted(ctx context.Context, client kubernetes.Interface, namespace, serviceName, clientName string) error {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		_, serverErr := client.CoreV1().Pods(namespace).Get(ctx, serviceName, metav1.GetOptions{})
		_, clientErr := client.CoreV1().Pods(namespace).Get(ctx, clientName, metav1.GetOptions{})
		_, serviceErr := client.CoreV1().Services(namespace).Get(ctx, serviceName, metav1.GetOptions{})
		if serverErr != nil && !apierrors.IsNotFound(serverErr) {
			return fmt.Errorf("check service probe server deletion: %w", serverErr)
		}
		if clientErr != nil && !apierrors.IsNotFound(clientErr) {
			return fmt.Errorf("check service probe client deletion: %w", clientErr)
		}
		if serviceErr != nil && !apierrors.IsNotFound(serviceErr) {
			return fmt.Errorf("check service probe Service deletion: %w", serviceErr)
		}
		if apierrors.IsNotFound(serverErr) && apierrors.IsNotFound(clientErr) && apierrors.IsNotFound(serviceErr) {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("timed out waiting for previous service probe resources to be deleted")
		case <-ticker.C:
		}
	}
}

func stringPtr(value string) *string { return &value }

func isNodeReady(node *corev1.Node) bool {
	if node == nil {
		return false
	}
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady && condition.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func flannelPodReady(pod *corev1.Pod) bool {
	if pod == nil || pod.Status.Phase != corev1.PodRunning {
		return false
	}
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}

func (d *NodeDeployer) apiserverVersion() (string, error) {
	if d.restConfig == nil {
		return "", fmt.Errorf("apiserver rest config is required")
	}
	client, err := kubernetes.NewForConfig(d.restConfig)
	if err != nil {
		return "", err
	}
	info, err := client.Discovery().ServerVersion()
	if err != nil {
		return "", err
	}
	version := info.GitVersion
	if version == "" {
		return "", fmt.Errorf("apiserver returned empty version")
	}
	// Strip distro suffixes like "-k3s1", "-eks-1" so the version maps to a
	// valid dl.k8s.io release path (e.g. "v1.36.1-k3s1" → "v1.36.1").
	if idx := strings.Index(version[1:], "-"); idx != -1 {
		version = version[:idx+1]
	}
	return version, nil
}

func deploymentWaitDeadline(ctx context.Context) (*time.Timer, <-chan time.Time) {
	if deadline, ok := ctx.Deadline(); ok {
		duration := time.Until(deadline)
		if duration <= 0 {
			timer := time.NewTimer(0)
			return timer, timer.C
		}
		timer := time.NewTimer(duration)
		return timer, timer.C
	}
	timer := time.NewTimer(4 * time.Minute)
	return timer, timer.C
}
