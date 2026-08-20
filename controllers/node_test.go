package controllers

import (
	"testing"
	"time"

	"github.com/casosorg/casos/server"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func nodeWithReady(status corev1.ConditionStatus, reason, message string) corev1.Node {
	return corev1.Node{
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{
					Type:              corev1.NodeReady,
					Status:            status,
					Reason:            reason,
					Message:           message,
					LastHeartbeatTime: metav1.NewTime(time.Date(2026, 8, 18, 10, 30, 43, 0, time.UTC)),
				},
			},
		},
	}
}

func TestToNodeSummaryReadyHasNoReason(t *testing.T) {
	got := toNodeSummary(nodeWithReady(corev1.ConditionTrue, "KubeletReady", "kubelet is posting ready status"))
	if got.Status != "Ready" {
		t.Fatalf("status = %q, want Ready", got.Status)
	}
	if got.StatusReason != "" || got.StatusMessage != "" {
		t.Errorf("healthy node carried reason %q / message %q, want both empty", got.StatusReason, got.StatusMessage)
	}
	if got.LastHeartbeat != "2026-08-18 10:30:43" {
		t.Errorf("lastHeartbeat = %q", got.LastHeartbeat)
	}
}

func TestToNodeSummaryUnknownCarriesReason(t *testing.T) {
	got := toNodeSummary(nodeWithReady(corev1.ConditionUnknown, "NodeStatusUnknown", "Kubelet stopped posting node status."))
	if got.Status != "NotReady" {
		t.Fatalf("status = %q, want NotReady", got.Status)
	}
	if got.StatusReason != "NodeStatusUnknown" {
		t.Errorf("statusReason = %q", got.StatusReason)
	}
	if got.StatusMessage != "Kubelet stopped posting node status." {
		t.Errorf("statusMessage = %q", got.StatusMessage)
	}
}

func TestToNodeSummaryFalseCarriesReason(t *testing.T) {
	got := toNodeSummary(nodeWithReady(corev1.ConditionFalse, "KubeletNotReady", "container runtime network not ready"))
	if got.Status != "NotReady" || got.StatusReason != "KubeletNotReady" {
		t.Fatalf("got status %q reason %q", got.Status, got.StatusReason)
	}
}

func TestToNodeSummaryWithoutReadyCondition(t *testing.T) {
	got := toNodeSummary(corev1.Node{})
	if got.Status != "Unknown" {
		t.Fatalf("status = %q, want Unknown", got.Status)
	}
	if got.StatusReason != "NoReadyCondition" {
		t.Errorf("statusReason = %q, want NoReadyCondition", got.StatusReason)
	}
	if got.StatusMessage == "" {
		t.Error("statusMessage is empty, want an explanation")
	}
}

func TestAttachAdmissionDenialsOnNotReadyNode(t *testing.T) {
	server.ClearAdmissionDenials()
	t.Cleanup(server.ClearAdmissionDenials)

	node := nodeWithReady(corev1.ConditionUnknown, "NodeStatusUnknown", "Kubelet stopped posting node status.")
	node.Name = "wsl-ubuntu"
	summary := toNodeSummary(node)

	attachAdmissionDenials(&summary)
	if summary.AdmissionDenials != nil {
		t.Fatalf("got %+v with nothing recorded, want nil", summary.AdmissionDenials)
	}

	server.RecordAdmissionDenial("system:node:wsl-ubuntu", "kube-node-lease", "leases", "UPDATE",
		`denied by Casbin admission policy: no allow rule matches sub="system:node:wsl-ubuntu" ns="kube-node-lease" resource="leases" action="UPDATE"`)
	attachAdmissionDenials(&summary)
	if len(summary.AdmissionDenials) != 1 {
		t.Fatalf("got %d denials, want the one recorded for this node's kubelet", len(summary.AdmissionDenials))
	}
	if summary.AdmissionDenials[0].Resource != "leases" {
		t.Errorf("Resource = %q, want leases", summary.AdmissionDenials[0].Resource)
	}
}

func TestAttachAdmissionDenialsSkipsReadyNode(t *testing.T) {
	server.ClearAdmissionDenials()
	t.Cleanup(server.ClearAdmissionDenials)

	server.RecordAdmissionDenial("system:node:wsl-ubuntu", "kube-node-lease", "leases", "UPDATE",
		`denied by Casbin admission policy: no allow rule matches sub="system:node:wsl-ubuntu" ns="kube-node-lease" resource="leases" action="UPDATE"`)

	node := nodeWithReady(corev1.ConditionTrue, "KubeletReady", "kubelet is posting ready status")
	node.Name = "wsl-ubuntu"
	summary := toNodeSummary(node)
	attachAdmissionDenials(&summary)

	if summary.AdmissionDenials != nil {
		t.Errorf("got %+v on a Ready node, want nil", summary.AdmissionDenials)
	}
}
