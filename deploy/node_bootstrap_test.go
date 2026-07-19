package deploy

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/casosorg/casos/object"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"
)

func TestProbeNamesAreScopedToValidationRun(t *testing.T) {
	for name, makeName := range map[string]func(string, string) string{
		"storage":   storageProbeName,
		"scheduler": schedulerProbeName,
		"service":   serviceProbeName,
	} {
		t.Run(name, func(t *testing.T) {
			first := makeName("worker-1", "task-1")
			second := makeName("worker-1", "task-2")
			if first == second {
				t.Fatalf("probe names collide across validation runs: %q", first)
			}
			if len(first) > 63 || len(second) > 63 {
				t.Fatalf("probe name exceeds DNS label limit: %q / %q", first, second)
			}
		})
	}
}

func TestNodeDeployApiserverProbeStatus(t *testing.T) {
	tests := map[string]bool{
		"200":  true,
		"204":  true,
		"401":  true,
		"403":  true,
		"400":  false,
		"404":  false,
		"500":  false,
		"503":  false,
		"":     false,
		"20":   false,
		"2000": false,
	}
	for status, expected := range tests {
		if got := isNodeDeployApiserverProbeStatus(status); got != expected {
			t.Errorf("status %q: expected %t, got %t", status, expected, got)
		}
	}
}

func TestAnyReadyProbePodIgnoresStaleUnreadyPod(t *testing.T) {
	pods := []corev1.Pod{
		{ObjectMeta: metav1.ObjectMeta{Name: "stale"}, Status: corev1.PodStatus{Phase: corev1.PodFailed}},
		{ObjectMeta: metav1.ObjectMeta{Name: "current"}, Status: corev1.PodStatus{Phase: corev1.PodRunning, Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}}},
	}
	if !anyReadyPod(pods) {
		t.Fatal("expected the ready non-terminating Pod to satisfy readiness")
	}
}

func TestDeleteStorageProbeResourcesReturnsDeleteErrors(t *testing.T) {
	client := fake.NewSimpleClientset()
	client.PrependReactor("delete", "pods", func(ktesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("apiserver unavailable")
	})
	if err := deleteStorageProbeResources(context.Background(), client, "kube-system", "probe"); err == nil {
		t.Fatal("expected cleanup failure to be observable")
	}
}

func TestMachineNodeDeployCompletionFailsWhenMachineStatusUpdateFails(t *testing.T) {
	success, phase, message := machineNodeDeployCompletion(errors.New("database unavailable"))
	if success {
		t.Fatal("expected task completion to fail when machine status persistence fails")
	}
	if phase != object.MachineNodeDeployPhaseFailed {
		t.Fatalf("expected failed phase, got %q", phase)
	}
	if !strings.Contains(message, "database unavailable") {
		t.Fatalf("expected persistence error in task message, got %q", message)
	}
}

func TestServiceProbeClientCommandChecksClusterDNS(t *testing.T) {
	command := serviceProbeClientCommand("10.244.1.2", "10.96.0.42")
	if !strings.Contains(command, "nslookup kubernetes.default.svc") {
		t.Fatalf("service probe does not verify cluster DNS: %q", command)
	}
}

func TestWorkerBootstrapTaintIsIdempotentAndPreservesOtherTaints(t *testing.T) {
	node := &corev1.Node{Spec: corev1.NodeSpec{Taints: []corev1.Taint{{
		Key: "dedicated", Value: "worker", Effect: corev1.TaintEffectNoSchedule,
	}}}}
	if !ensureWorkerBootstrapTaint(node) {
		t.Fatal("expected the bootstrap taint to be added")
	}
	if ensureWorkerBootstrapTaint(node) {
		t.Fatal("expected applying the same bootstrap taint to be idempotent")
	}
	if len(node.Spec.Taints) != 2 || node.Spec.Taints[0].Key != "dedicated" || node.Spec.Taints[1].Key != workerBootstrapTaintKey {
		t.Fatalf("unexpected taints after reconciliation: %#v", node.Spec.Taints)
	}
}

func TestServiceProbeUsesAnotherReadyWorkerWhenAvailable(t *testing.T) {
	ready := []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}
	nodes := []corev1.Node{
		{ObjectMeta: metav1.ObjectMeta{Name: "target", Labels: map[string]string{corev1.LabelHostname: "target-host"}}, Spec: corev1.NodeSpec{PodCIDR: "10.244.1.0/24"}, Status: corev1.NodeStatus{Conditions: ready}},
		{ObjectMeta: metav1.ObjectMeta{Name: "peer", Labels: map[string]string{corev1.LabelHostname: "peer-host"}}, Spec: corev1.NodeSpec{PodCIDRs: []string{"10.244.2.0/24"}}, Status: corev1.NodeStatus{Conditions: ready}},
	}
	placement, err := selectServiceProbePlacement(nodes, "target")
	if err != nil {
		t.Fatalf("select placement: %v", err)
	}
	if placement.clientHostname != "target-host" || placement.serverHostname != "peer-host" {
		t.Fatalf("expected cross-worker service validation, got %#v", placement)
	}
}
