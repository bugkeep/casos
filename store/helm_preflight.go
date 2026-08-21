package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	helmInstallPreflightTimeout = 10 * time.Second
	// A node that only just went NotReady is probably still joining, so the
	// install waits it out instead of failing a cluster that is coming up.
	helmInstallNodeSettleGrace = 2 * time.Minute
)

// validateHelmInstallNodes fails an install that Helm's Wait could only end by
// timing out: nothing schedulable exists, so the release's Pods stay Pending for
// the whole helmInstallTimeout. It rejects only states that cannot resolve on
// their own; anything inconclusive degrades to a warning, like getClusterNodeIPs.
func validateHelmInstallNodes(parent context.Context, cfg *rest.Config, warn func(string)) error {
	if cfg == nil || warn == nil {
		return nil
	}
	if parent == nil {
		parent = context.Background()
	}
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		warn(fmt.Sprintf("skipped the node preflight check: %v", err))
		return nil
	}
	ctx, cancel := context.WithTimeout(parent, helmInstallPreflightTimeout)
	defer cancel()
	nodes, err := client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		warn(fmt.Sprintf("skipped the node preflight check: %v", err))
		return nil
	}

	problem, settling := helmInstallNodeProblem(nodes.Items, time.Now())
	switch {
	case problem == "":
		return nil
	case settling:
		warn(problem)
		return nil
	default:
		return errors.New(problem)
	}
}

func helmInstallNodeProblem(nodes []corev1.Node, now time.Time) (problem string, settling bool) {
	if len(nodes) == 0 {
		return "no Kubernetes nodes are registered, so this app has nowhere to run; add a worker node before installing", false
	}
	cordoned, notReady := 0, 0
	schedulable := 0
	var schedulableAllocatablePods int64
	anySchedulableReady := false
	for _, node := range nodes {
		if node.Spec.Unschedulable {
			cordoned++
			continue
		}
		schedulable++
		schedulableAllocatablePods += nodeAllocatablePods(node)
		ready, since := helmInstallNodeReadiness(node)
		if ready {
			anySchedulableReady = true
			continue
		}
		notReady++
		if since.IsZero() || now.Sub(since) < helmInstallNodeSettleGrace {
			settling = true
		}
	}
	// Branch 1 — fast pass: at least one schedulable node is Ready AND the
	// cluster reports non-zero Pod capacity overall. Helm can attempt to
	// schedule; any notReady node's settling state is irrelevant.
	if anySchedulableReady && schedulableAllocatablePods > 0 {
		return "", false
	}
	// Branch 2 — resource-exhaustion guard. At least one schedulable node is
	// Ready (so kubelet is responding) yet the cluster reports zero
	// allocatable Pods overall. The release's Pods would stay Pending for
	// the full helmInstallTimeout; fail in seconds instead. This branch
	// intentionally hard-fails even when other nodes are settling, because
	// the Ready node's reported state is the authoritative signal — kubelet
	// writes Conditions (including Ready) and Allocatable in the same
	// NodeStatus update, so a Ready=True + pods=0 observation is the
	// genuine cluster state, not a transient boot window.
	if anySchedulableReady && schedulableAllocatablePods == 0 {
		return fmt.Sprintf(
			"no Kubernetes node can fit a Pod (%d schedulable, total pods=0 allocatable); free capacity or add workers before installing",
			schedulable,
		), false
	}
	// Branch 3 — no Ready node. The readiness summary dominates; a pods=0
	// observation here is incidental (kubelet has not reported yet, or the
	// cluster is mid-boot), not the root cause. Preserve settling so a
	// cold-start cluster can still install.
	return fmt.Sprintf(
		"no Kubernetes node can run this app (%d cordoned, %d not ready); uncordon or repair a worker node before installing",
		cordoned, notReady,
	), settling
}

func helmInstallNodeReadiness(node corev1.Node) (bool, time.Time) {
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady {
			return condition.Status == corev1.ConditionTrue, condition.LastTransitionTime.Time
		}
	}
	return false, time.Time{}
}

// nodeAllocatablePods returns the Pod capacity that a schedulable node is
// willing to admit, or 0 if the node reports no Pods key at all (treated as
// "cannot accept work" rather than infinity).
func nodeAllocatablePods(node corev1.Node) int64 {
	q, ok := node.Status.Allocatable[corev1.ResourcePods]
	if !ok {
		return 0
	}
	return q.Value()
}
