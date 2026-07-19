package controllers

import (
	"testing"

	networkingv1 "k8s.io/api/networking/v1"
)

func TestIngressSummaryIncludesPublishedAddresses(t *testing.T) {
	summary := toIngressSummary(networkingv1.Ingress{Status: networkingv1.IngressStatus{LoadBalancer: networkingv1.IngressLoadBalancerStatus{
		Ingress: []networkingv1.IngressLoadBalancerIngress{{IP: "192.0.2.10"}, {Hostname: "ingress.example.test"}},
	}}})
	if len(summary.LoadBalancerAddresses) != 2 || summary.LoadBalancerAddresses[0] != "192.0.2.10" || summary.LoadBalancerAddresses[1] != "ingress.example.test" {
		t.Fatalf("expected published Ingress addresses, got %v", summary.LoadBalancerAddresses)
	}
}
