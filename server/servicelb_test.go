package server

import (
	"context"
	"errors"
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"
)

func TestServiceLBSkipsForeignLoadBalancerClass(t *testing.T) {
	foreignClass := "metallb.io/controller"
	ready := corev1.ConditionTrue
	client := fake.NewSimpleClientset(
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "worker"}, Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: ready}},
			Addresses:  []corev1.NodeAddress{{Type: corev1.NodeInternalIP, Address: "192.0.2.10"}},
		}},
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "default"}, Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeLoadBalancer, LoadBalancerClass: &foreignClass,
		}},
	)
	if err := reconcileServiceLB(context.Background(), client); err != nil {
		t.Fatalf("reconcile service LB: %v", err)
	}
	stored, _ := client.CoreV1().Services("default").Get(context.Background(), "app", metav1.GetOptions{})
	if len(stored.Spec.ExternalIPs) != 0 || len(stored.Status.LoadBalancer.Ingress) != 0 || stored.Annotations[serviceLBManagedIPsAnnotation] != "" {
		t.Fatalf("foreign LoadBalancer service was modified: annotations=%v spec=%v status=%v", stored.Annotations, stored.Spec.ExternalIPs, stored.Status.LoadBalancer.Ingress)
	}
}

func TestServiceLBSkipsUnclassedServiceWithoutOptIn(t *testing.T) {
	service := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "default"}, Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeLoadBalancer}}
	client := fake.NewSimpleClientset(service)
	if err := reconcileServiceLB(context.Background(), client); err != nil {
		t.Fatalf("reconcile service LB: %v", err)
	}
	stored, _ := client.CoreV1().Services("default").Get(context.Background(), "app", metav1.GetOptions{})
	if stored.Annotations[serviceLBManagedIPsAnnotation] != "" {
		t.Fatalf("unclassed Service was claimed without opt-in: %v", stored.Annotations)
	}
}

func TestServiceLBDisabledAnnotationCleansManagedState(t *testing.T) {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "default", Annotations: map[string]string{
			serviceLBDisabledAnnotation:   "true",
			serviceLBManagedIPsAnnotation: `["192.0.2.10"]`,
		}},
		Spec:   corev1.ServiceSpec{Type: corev1.ServiceTypeLoadBalancer, ExternalIPs: []string{"198.51.100.5", "192.0.2.10"}},
		Status: corev1.ServiceStatus{LoadBalancer: corev1.LoadBalancerStatus{Ingress: []corev1.LoadBalancerIngress{{IP: "192.0.2.10"}}}},
	}
	client := fake.NewSimpleClientset(service)
	if err := reconcileServiceLB(context.Background(), client); err != nil {
		t.Fatalf("reconcile service LB: %v", err)
	}
	stored, _ := client.CoreV1().Services("default").Get(context.Background(), "app", metav1.GetOptions{})
	if len(stored.Spec.ExternalIPs) != 1 || stored.Spec.ExternalIPs[0] != "198.51.100.5" || stored.Annotations[serviceLBManagedIPsAnnotation] != "" || len(stored.Status.LoadBalancer.Ingress) != 0 {
		t.Fatalf("managed state was not cleaned: annotations=%v externalIPs=%v status=%v", stored.Annotations, stored.Spec.ExternalIPs, stored.Status.LoadBalancer.Ingress)
	}
}

func TestCleanupServiceLBPreservesUserState(t *testing.T) {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "default", Annotations: map[string]string{
			serviceLBManagedIPsAnnotation: `["192.0.2.10"]`,
		}},
		Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeLoadBalancer, ExternalIPs: []string{"198.51.100.5", "192.0.2.10"}},
		Status: corev1.ServiceStatus{LoadBalancer: corev1.LoadBalancerStatus{Ingress: []corev1.LoadBalancerIngress{
			{IP: "192.0.2.10"}, {Hostname: "external.example.com"},
		}}},
	}
	client := fake.NewSimpleClientset(service)
	if err := cleanupServiceLB(context.Background(), client); err != nil {
		t.Fatalf("cleanup ServiceLB: %v", err)
	}
	stored, _ := client.CoreV1().Services("default").Get(context.Background(), "app", metav1.GetOptions{})
	wantStatus := []corev1.LoadBalancerIngress{{Hostname: "external.example.com"}}
	if !reflect.DeepEqual(stored.Spec.ExternalIPs, []string{"198.51.100.5"}) || !reflect.DeepEqual(stored.Status.LoadBalancer.Ingress, wantStatus) || stored.Annotations[serviceLBManagedIPsAnnotation] != "" {
		t.Fatalf("cleanup removed user state: annotations=%v externalIPs=%v status=%v", stored.Annotations, stored.Spec.ExternalIPs, stored.Status.LoadBalancer.Ingress)
	}
}

func TestCleanupServiceLBKeepsOwnershipWhenStatusUpdateFails(t *testing.T) {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "default", Annotations: map[string]string{
			serviceLBManagedIPsAnnotation: `["192.0.2.10"]`,
		}},
		Spec:   corev1.ServiceSpec{Type: corev1.ServiceTypeLoadBalancer, ExternalIPs: []string{"192.0.2.10"}},
		Status: corev1.ServiceStatus{LoadBalancer: corev1.LoadBalancerStatus{Ingress: []corev1.LoadBalancerIngress{{IP: "192.0.2.10"}}}},
	}
	client := fake.NewSimpleClientset(service)
	client.PrependReactor("update", "services", func(action ktesting.Action) (bool, runtime.Object, error) {
		if action.GetSubresource() == "status" {
			return true, nil, errors.New("status update failed")
		}
		return false, nil, nil
	})
	if err := cleanupLoadBalancerService(context.Background(), client, service); err == nil {
		t.Fatal("expected status update failure")
	}
	stored, _ := client.CoreV1().Services("default").Get(context.Background(), "app", metav1.GetOptions{})
	if stored.Annotations[serviceLBManagedIPsAnnotation] == "" || len(stored.Spec.ExternalIPs) != 1 {
		t.Fatalf("cleanup lost ownership after partial failure: annotations=%v externalIPs=%v", stored.Annotations, stored.Spec.ExternalIPs)
	}
}

func TestServiceLBDoesNotPublishControlPlaneOnlyNodes(t *testing.T) {
	ready := corev1.ConditionTrue
	nodes := []corev1.Node{{
		ObjectMeta: metav1.ObjectMeta{Name: "control-plane", Labels: map[string]string{"node-role.kubernetes.io/control-plane": ""}},
		Status:     corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: ready}}, Addresses: []corev1.NodeAddress{{Type: corev1.NodeInternalIP, Address: "192.0.2.20"}}},
	}}
	if selected := readyServiceLBNodes(nodes); len(selected) != 0 {
		t.Fatalf("control-plane nodes were published without explicit support: %v", selected)
	}
}

func TestReconcileLoadBalancerServiceRetriesConflict(t *testing.T) {
	class := serviceLBClass
	service := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "default"}, Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeLoadBalancer, LoadBalancerClass: &class}}
	client := fake.NewSimpleClientset(service)
	conflicts := 0
	client.PrependReactor("update", "services", func(action ktesting.Action) (bool, runtime.Object, error) {
		if action.GetSubresource() == "" && conflicts == 0 {
			conflicts++
			return true, nil, apierrors.NewConflict(schema.GroupResource{Resource: "services"}, "app", nil)
		}
		return false, nil, nil
	})
	if err := reconcileLoadBalancerService(context.Background(), client, service, []string{"192.0.2.10"}); err != nil {
		t.Fatalf("reconcile after conflict: %v", err)
	}
	stored, _ := client.CoreV1().Services("default").Get(context.Background(), "app", metav1.GetOptions{})
	if conflicts != 1 || len(stored.Spec.ExternalIPs) != 1 || stored.Spec.ExternalIPs[0] != "192.0.2.10" {
		t.Fatalf("conflict was not retried: conflicts=%d externalIPs=%v", conflicts, stored.Spec.ExternalIPs)
	}
}

func TestReconcileLoadBalancerServicePreservesUnmanagedStatus(t *testing.T) {
	class := serviceLBClass
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "default", Annotations: map[string]string{
			serviceLBManagedIPsAnnotation: `["192.0.2.9"]`,
		}},
		Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeLoadBalancer, LoadBalancerClass: &class, ExternalIPs: []string{"192.0.2.9"}},
		Status: corev1.ServiceStatus{LoadBalancer: corev1.LoadBalancerStatus{Ingress: []corev1.LoadBalancerIngress{
			{IP: "192.0.2.9"},
			{Hostname: "external.example.com"},
		}}},
	}
	client := fake.NewSimpleClientset(service)
	if err := reconcileLoadBalancerService(context.Background(), client, service, []string{"192.0.2.10"}); err != nil {
		t.Fatalf("reconcile LoadBalancer service: %v", err)
	}
	stored, _ := client.CoreV1().Services("default").Get(context.Background(), "app", metav1.GetOptions{})
	want := []corev1.LoadBalancerIngress{{Hostname: "external.example.com"}, {IP: "192.0.2.10"}}
	if !reflect.DeepEqual(stored.Status.LoadBalancer.Ingress, want) {
		t.Fatalf("unmanaged status was not preserved: %v", stored.Status.LoadBalancer.Ingress)
	}
}

func TestReconcileLoadBalancerServiceSkipsDeletingService(t *testing.T) {
	class := serviceLBClass
	now := metav1.Now()
	service := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "default", DeletionTimestamp: &now}, Spec: corev1.ServiceSpec{
		Type: corev1.ServiceTypeLoadBalancer, LoadBalancerClass: &class,
	}}
	client := fake.NewSimpleClientset(service)
	if err := reconcileLoadBalancerService(context.Background(), client, service, []string{"192.0.2.10"}); err != nil {
		t.Fatalf("reconcile deleting service: %v", err)
	}
	stored, _ := client.CoreV1().Services("default").Get(context.Background(), "app", metav1.GetOptions{})
	if len(stored.Spec.ExternalIPs) != 0 || len(stored.Status.LoadBalancer.Ingress) != 0 {
		t.Fatalf("deleting service was modified: spec=%v status=%v", stored.Spec.ExternalIPs, stored.Status.LoadBalancer.Ingress)
	}
}

func TestReadyServiceLBNodesPreferExternalIPAndSkipCordonedNodes(t *testing.T) {
	ready := corev1.ConditionTrue
	nodes := []corev1.Node{
		{ObjectMeta: metav1.ObjectMeta{Name: "public-worker"}, Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: ready}},
			Addresses:  []corev1.NodeAddress{{Type: corev1.NodeInternalIP, Address: "10.0.0.10"}, {Type: corev1.NodeExternalIP, Address: "192.0.2.10"}},
		}},
		{ObjectMeta: metav1.ObjectMeta{Name: "cordoned-worker"}, Spec: corev1.NodeSpec{Unschedulable: true}, Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: ready}},
			Addresses:  []corev1.NodeAddress{{Type: corev1.NodeExternalIP, Address: "192.0.2.11"}},
		}},
	}
	selected := readyServiceLBNodes(nodes)
	if len(selected) != 1 || len(selected[0].ips) != 1 || selected[0].ips[0] != "192.0.2.10" {
		t.Fatalf("unexpected selected nodes: %v", selected)
	}
}

func TestLocalServiceLBPrefersWorkerEndpointOverControlPlane(t *testing.T) {
	ready := corev1.ConditionTrue
	workerName, controlPlaneName := "worker", "control-plane"
	service := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "default"}, Spec: corev1.ServiceSpec{
		Type: corev1.ServiceTypeLoadBalancer, ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyTypeLocal,
	}}
	client := fake.NewSimpleClientset(&discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "default", Labels: map[string]string{discoveryv1.LabelServiceName: "app"}},
		Endpoints:  []discoveryv1.Endpoint{{NodeName: &workerName}, {NodeName: &controlPlaneName}},
	})
	nodes := []corev1.Node{
		{ObjectMeta: metav1.ObjectMeta{Name: workerName}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: ready}}, Addresses: []corev1.NodeAddress{{Type: corev1.NodeInternalIP, Address: "192.0.2.10"}}}},
		{ObjectMeta: metav1.ObjectMeta{Name: controlPlaneName, Labels: map[string]string{"node-role.kubernetes.io/control-plane": ""}}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: ready}}, Addresses: []corev1.NodeAddress{{Type: corev1.NodeInternalIP, Address: "192.0.2.20"}}}},
	}
	ips, err := serviceLBNodeIPs(context.Background(), client, service, nodes)
	if err != nil {
		t.Fatalf("select ServiceLB IPs: %v", err)
	}
	if len(ips) != 1 || ips[0] != "192.0.2.10" {
		t.Fatalf("expected only worker IP, got %v", ips)
	}
}
