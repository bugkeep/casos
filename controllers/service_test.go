package controllers

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestServiceSummaryIncludesLoadBalancerHostname(t *testing.T) {
	summary := toSvcSummary(corev1.Service{Status: corev1.ServiceStatus{LoadBalancer: corev1.LoadBalancerStatus{
		Ingress: []corev1.LoadBalancerIngress{{Hostname: "lb.example.test"}},
	}}})
	if len(summary.LoadBalancerAddresses) != 1 || summary.LoadBalancerAddresses[0] != "lb.example.test" {
		t.Fatalf("expected LoadBalancer hostname, got %v", summary.LoadBalancerAddresses)
	}
}
