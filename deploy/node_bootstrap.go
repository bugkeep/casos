package deploy

import (
	"context"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/retry"
)

type nodeBootstrapState struct {
	podCIDR string
	ready   bool
}

const (
	workerBootstrapTaintKey   = "casos.io/bootstrap"
	workerBootstrapTaintValue = "platform"
)

type NodeDeployer struct {
	config     Config
	restConfig *rest.Config
	log        NodeDeployLogger
}

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
	if err = d.ensureNodeBootstrapTaint(ctx, opts.NodeName); err != nil {
		return nil, fmt.Errorf("protect worker during bootstrap: %w", err)
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
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline:
			return fmt.Errorf("timed out waiting for Flannel to become Ready on worker %s", nodeName)
		case <-ticker.C:
			pods, err := client.CoreV1().Pods("kube-flannel").List(ctx, metav1.ListOptions{
				LabelSelector: "k8s-app=flannel",
				FieldSelector: "spec.nodeName=" + nodeName,
			})
			if err != nil {
				return err
			}
			for i := range pods.Items {
				if flannelPodReady(&pods.Items[i]) {
					return nil
				}
			}
		}
	}
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
			updated, updateErr := ensureWorkerBootstrapTaintOnCluster(ctx, client, nodeName)
			if updateErr != nil {
				return nil, fmt.Errorf("ensure worker bootstrap taint: %w", updateErr)
			}
			node = updated
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

// WaitForOperational keeps a worker unschedulable until the platform pieces
// needed by ordinary workloads are actually available.
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
	lastReason := "platform prerequisites are not ready"
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline:
			return fmt.Errorf("timed out waiting for worker operational readiness: %s", lastReason)
		case <-ticker.C:
			reason, ready, err := workerOperationalState(ctx, client, nodeName, d.config.StorageProvisionerEnabled)
			if err != nil {
				return err
			}
			if ready {
				if err := d.removeNodeBootstrapTaint(ctx, nodeName); err != nil {
					return fmt.Errorf("finish worker bootstrap: %w", err)
				}
				return nil
			}
			lastReason = reason
		}
	}
}

func ensureWorkerBootstrapTaint(node *corev1.Node) (bool, error) {
	for _, taint := range node.Spec.Taints {
		if taint.Key != workerBootstrapTaintKey || taint.Effect != corev1.TaintEffectNoSchedule {
			continue
		}
		if taint.Value != workerBootstrapTaintValue {
			return false, fmt.Errorf("node has conflicting %s taint value %q", workerBootstrapTaintKey, taint.Value)
		}
		return false, nil
	}
	node.Spec.Taints = append(node.Spec.Taints, corev1.Taint{Key: workerBootstrapTaintKey, Value: workerBootstrapTaintValue, Effect: corev1.TaintEffectNoSchedule})
	return true, nil
}

func removeWorkerBootstrapTaint(node *corev1.Node) bool {
	filtered := node.Spec.Taints[:0]
	removed := false
	for _, taint := range node.Spec.Taints {
		if taint.Key == workerBootstrapTaintKey && taint.Value == workerBootstrapTaintValue && taint.Effect == corev1.TaintEffectNoSchedule {
			removed = true
			continue
		}
		filtered = append(filtered, taint)
	}
	if removed {
		node.Spec.Taints = filtered
	}
	return removed
}

func ensureWorkerBootstrapTaintOnCluster(ctx context.Context, client kubernetes.Interface, nodeName string) (*corev1.Node, error) {
	var result *corev1.Node
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		node, err := client.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		changed, err := ensureWorkerBootstrapTaint(node)
		if err != nil {
			return err
		}
		if !changed {
			result = node
			return nil
		}
		result, err = client.CoreV1().Nodes().Update(ctx, node, metav1.UpdateOptions{})
		return err
	})
	return result, err
}

func (d *NodeDeployer) ensureNodeBootstrapTaint(ctx context.Context, nodeName string) error {
	client, err := kubernetes.NewForConfig(d.restConfig)
	if err != nil {
		return err
	}
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		node, err := client.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			node = &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: nodeName}}
			if _, err := ensureWorkerBootstrapTaint(node); err != nil {
				return err
			}
			_, err = client.CoreV1().Nodes().Create(ctx, node, metav1.CreateOptions{})
			if apierrors.IsAlreadyExists(err) {
				return apierrors.NewConflict(schema.GroupResource{Resource: "nodes"}, nodeName, err)
			}
			return err
		}
		if err != nil {
			return err
		}
		changed, err := ensureWorkerBootstrapTaint(node)
		if err != nil {
			return err
		}
		if !changed {
			return nil
		}
		_, err = client.CoreV1().Nodes().Update(ctx, node, metav1.UpdateOptions{})
		return err
	})
}

func (d *NodeDeployer) removeNodeBootstrapTaint(ctx context.Context, nodeName string) error {
	client, err := kubernetes.NewForConfig(d.restConfig)
	if err != nil {
		return err
	}
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		node, err := client.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if !removeWorkerBootstrapTaint(node) {
			return nil
		}
		_, err = client.CoreV1().Nodes().Update(ctx, node, metav1.UpdateOptions{})
		return err
	})
}

func workerOperationalState(ctx context.Context, client kubernetes.Interface, nodeName string, storageRequired bool) (string, bool, error) {
	node, err := client.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return "node is not registered", false, nil
	}
	if err != nil {
		return "", false, err
	}
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeNetworkUnavailable && condition.Status == corev1.ConditionTrue {
			return "node network is unavailable", false, nil
		}
	}
	if !isNodeReady(node) {
		return "node is not Ready", false, nil
	}
	if node.Spec.PodCIDR == "" {
		return "node has no PodCIDR", false, nil
	}
	flannelPods, err := client.CoreV1().Pods("kube-flannel").List(ctx, metav1.ListOptions{LabelSelector: "k8s-app=flannel", FieldSelector: "spec.nodeName=" + nodeName})
	if err != nil {
		return "", false, err
	}
	if len(flannelPods.Items) == 0 || !isPodReady(flannelPods.Items[0]) {
		return "Flannel is not Ready on the worker", false, nil
	}
	dns, err := client.AppsV1().Deployments("kube-system").Get(ctx, "coredns", metav1.GetOptions{})
	if apierrors.IsNotFound(err) || (err == nil && dns.Status.AvailableReplicas < 1) {
		return "CoreDNS is not Available", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if storageRequired {
		if _, err := client.StorageV1().StorageClasses().Get(ctx, "local-path", metav1.GetOptions{}); apierrors.IsNotFound(err) {
			return "default local-path StorageClass is missing", false, nil
		} else if err != nil {
			return "", false, err
		}
	}
	return "", true, nil
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
