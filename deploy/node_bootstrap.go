package deploy

import (
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
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

const flannelDaemonSetName = "kube-flannel-ds"
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
	if err = d.ensureNodeCIDR(ctx, opts.NodeName); err != nil {
		return nil, err
	}
	if err = d.startKubelet(ctx, runner); err != nil {
		return nil, err
	}

	d.logStep(nodeDeployPhaseWaiting, "Waiting for node registration")
	bootstrapState, err := d.waitForNodeBootstrapState(ctx, opts.NodeName)
	if err != nil {
		return nil, err
	}

	if err = d.startKubeProxy(ctx, runner); err != nil {
		return nil, err
	}

	if !bootstrapState.ready {
		d.logStep(nodeDeployPhaseWaiting, "Waiting for Node Ready")
		if err = d.waitForNodeReady(ctx, opts.NodeName); err != nil {
			return nil, err
		}
	} else {
		d.logStep(nodeDeployPhaseWaiting, "Node is already Ready")
	}

	d.logStep(nodeDeployPhaseWaiting, "Waiting for Flannel to become Ready on the worker")
	if err = d.waitForFlannelReady(ctx, opts.NodeName); err != nil {
		return nil, err
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

func (d *NodeDeployer) ensureNodeCIDR(ctx context.Context, nodeName string) error {
	if d.restConfig == nil {
		return fmt.Errorf("apiserver rest config is required")
	}
	client, err := kubernetes.NewForConfig(d.restConfig)
	if err != nil {
		return err
	}

	nodeCIDRReservationMu.Lock()
	defer nodeCIDRReservationMu.Unlock()

	nodes, err := client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("list nodes for PodCIDR reservation: %w", err)
	}
	for i := range nodes.Items {
		if nodes.Items[i].Name != nodeName {
			continue
		}
		cidr := nodeCIDRFromSpec(&nodes.Items[i])
		if cidr == "" {
			cidr, err = allocateNodeCIDR(nodes.Items)
			if err != nil {
				return err
			}
			nodes.Items[i].Spec.PodCIDR = cidr
			nodes.Items[i].Spec.PodCIDRs = []string{cidr}
			if _, err = client.CoreV1().Nodes().Update(ctx, &nodes.Items[i], metav1.UpdateOptions{}); err != nil {
				return fmt.Errorf("reserve PodCIDR for node %s: %w", nodeName, err)
			}
		}
		return nil
	}

	cidr, err := allocateNodeCIDR(nodes.Items)
	if err != nil {
		return err
	}
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   nodeName,
			Labels: map[string]string{corev1.LabelOSStable: "linux"},
		},
		Spec: corev1.NodeSpec{PodCIDR: cidr, PodCIDRs: []string{cidr}},
	}
	if _, err = client.CoreV1().Nodes().Create(ctx, node, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("create worker node with PodCIDR: %w", err)
	}
	return nil
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
				FieldSelector: "spec.nodeName=" + nodeName,
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
	return fmt.Sprintf("Flannel Pod has not been scheduled on %s (desired=%d current=%d ready=%d available=%d updated=%d)",
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
