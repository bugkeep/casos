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
	for _, node := range nodes {
		if node.Spec.Unschedulable {
			cordoned++
			continue
		}
		ready, since := helmInstallNodeReadiness(node)
		if ready {
			return "", false
		}
		notReady++
		if since.IsZero() || now.Sub(since) < helmInstallNodeSettleGrace {
			settling = true
		}
	}
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
