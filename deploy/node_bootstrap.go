package deploy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
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
	if apierrors.IsNotFound(err) || (err == nil && dns.Status.AvailableReplicas < 1) {
		return "CoreDNS is not Available", false, nil
	}
	if err != nil {
		return "", false, err
	}

	if _, err := client.StorageV1().StorageClasses().Get(ctx, "local-path", metav1.GetOptions{}); apierrors.IsNotFound(err) {
		return "default local-path StorageClass is missing", false, nil
	} else if err != nil {
		return "", false, err
	}
	if err := waitForStorageProbe(ctx, client, nodeName, storageProbeImage); err != nil {
		return err.Error(), false, nil
	}
	hostname := nodeName
	if node.Labels["kubernetes.io/hostname"] != "" {
		hostname = node.Labels["kubernetes.io/hostname"]
	}
	if err := waitForSchedulerProbe(ctx, client, nodeName, hostname, storageProbeImage); err != nil {
		return err.Error(), false, nil
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

func storageProbeName(nodeName string) string {
	digest := sha256.Sum256([]byte(nodeName))
	return "casos-storage-" + hex.EncodeToString(digest[:])[:16]
}

func schedulerProbeName(nodeName string) string {
	digest := sha256.Sum256([]byte(nodeName))
	return "casos-scheduler-" + hex.EncodeToString(digest[:])[:16]
}

func workerProbeImage(image string) string {
	if image == "" {
		return "docker.1ms.run/library/busybox:1.37.0"
	}
	return image
}

func waitForStorageProbe(ctx context.Context, client kubernetes.Interface, nodeName, image string) error {
	image = workerProbeImage(image)
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
			NodeName:      nodeName,
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
