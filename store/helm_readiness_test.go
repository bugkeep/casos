package store

import (
	"context"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
)

func TestReadinessOwnershipIsLimitedToEndpoints(t *testing.T) {
	for _, kind := range []string{"Pod", "Deployment", "StatefulSet", "DaemonSet", "Job", "PersistentVolumeClaim"} {
		if _, ok := helmReadinessResourceKinds[kind]; ok {
			t.Fatalf("CasOS duplicated Helm's native readiness ownership for %s", kind)
		}
	}
	for _, kind := range []string{"Service", "Ingress"} {
		if _, ok := helmReadinessResourceKinds[kind]; !ok {
			t.Fatalf("missing endpoint readiness ownership for %s", kind)
		}
	}
}

func TestValidateHelmReleaseEndpointReadiness(t *testing.T) {
	readyService := service("default", "app", map[string]string{"app": "demo"})
	className := "casos"
	ingress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "app"},
		Spec: networkingv1.IngressSpec{
			IngressClassName: &className,
			Rules: []networkingv1.IngressRule{{IngressRuleValue: networkingv1.IngressRuleValue{
				HTTP: &networkingv1.HTTPIngressRuleValue{Paths: []networkingv1.HTTPIngressPath{{
					Backend: networkingv1.IngressBackend{Service: &networkingv1.IngressServiceBackend{
						Name: "app", Port: networkingv1.ServiceBackendPort{Number: 80},
					}},
				}}},
			}}},
		},
	}
	objects := []runtime.Object{
		readyService,
		ingress,
		&networkingv1.IngressClass{ObjectMeta: metav1.ObjectMeta{Name: className}},
		endpointSlice("default", "app", nil, nil),
		// A failed historical Pod with the same release label is Helm's concern,
		// not a second CasOS readiness manager.
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "stale", Labels: map[string]string{"app.kubernetes.io/instance": "demo"}}, Status: corev1.PodStatus{Phase: corev1.PodFailed}},
	}
	client := fake.NewSimpleClientset(objects...)
	refs := []helmResourceRef{
		{kind: "Service", namespace: "default", name: "app"},
		{kind: "Ingress", namespace: "default", name: "app"},
	}
	if err := validateHelmReleaseResourcesWithRefs(context.Background(), client, "demo", "default", refs); err != nil {
		t.Fatalf("ready release was rejected: %v", err)
	}
}

func TestEndpointSliceReadinessBoundaries(t *testing.T) {
	falseValue := false
	trueValue := true
	for _, tc := range []struct {
		name        string
		ready       *bool
		terminating *bool
		want        bool
	}{
		{name: "nil ready means ready", want: true},
		{name: "explicit ready", ready: &trueValue, want: true},
		{name: "not ready", ready: &falseValue, want: false},
		{name: "terminating", terminating: &trueValue, want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service := service("default", "app", map[string]string{"app": "demo"})
			client := fake.NewSimpleClientset(service, endpointSlice("default", "app", tc.ready, tc.terminating))
			got, err := serviceHasReadyEndpointSlice(context.Background(), client, *service)
			if err != nil {
				t.Fatalf("check EndpointSlice: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got ready=%v, want %v", got, tc.want)
			}
		})
	}
}

func TestClusterIPServiceRequiresReadyEndpoint(t *testing.T) {
	if !serviceRequiresReadyEndpoint(*service("default", "app", map[string]string{"app": "demo"})) {
		t.Fatal("selector-backed ClusterIP Service was skipped")
	}
	if !serviceRequiresReadyEndpoint(*service("default", "app", nil)) {
		t.Fatal("selectorless Service with manual Endpoints was skipped")
	}
	external := service("default", "external", map[string]string{"unused": "value"})
	external.Spec.Type = corev1.ServiceTypeExternalName
	if serviceRequiresReadyEndpoint(*external) {
		t.Fatal("ExternalName Service unexpectedly required EndpointSlices")
	}
}

func TestReadinessRejectsNilDependencies(t *testing.T) {
	if err := waitForHelmReleaseResources(context.Background(), nil, "demo", "default"); err == nil {
		t.Fatal("nil REST config did not return an error")
	}
	if err := validateHelmReleaseResourcesWithRefs(context.Background(), nil, "demo", "default", nil); err == nil {
		t.Fatal("nil Kubernetes client did not return an error")
	}
}

func TestEndpointSliceUnavailableFallsBackToEndpoints(t *testing.T) {
	service := service("default", "app", map[string]string{"app": "demo"})
	legacy := &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "app"},
		Subsets:    []corev1.EndpointSubset{{Addresses: []corev1.EndpointAddress{{IP: "10.0.0.10"}}}},
	}
	client := fake.NewSimpleClientset(service, legacy)
	client.PrependReactor("list", "endpointslices", func(clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewNotFound(schema.GroupResource{Group: "discovery.k8s.io", Resource: "endpointslices"}, "")
	})
	ready, err := serviceHasReadyEndpointSlice(context.Background(), client, *service)
	if err != nil {
		t.Fatalf("fallback to legacy Endpoints: %v", err)
	}
	if !ready {
		t.Fatal("ready legacy Endpoints were ignored")
	}
}

func TestMissingReleaseServiceIsNotReady(t *testing.T) {
	client := fake.NewSimpleClientset()
	err := validateHelmReleaseResourcesWithRefs(context.Background(), client, "demo", "default", []helmResourceRef{{
		kind: "Service", namespace: "default", name: "missing",
	}})
	if err == nil || !strings.Contains(err.Error(), "Service default/missing is missing") {
		t.Fatalf("unexpected readiness result: %v", err)
	}
}

func service(namespace, name string, selector map[string]string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: selector,
			Ports:    []corev1.ServicePort{{Port: 80, TargetPort: intstr.FromInt32(8080)}},
		},
	}
}

func endpointSlice(namespace, serviceName string, ready, terminating *bool) *discoveryv1.EndpointSlice {
	return &discoveryv1.EndpointSlice{
		ObjectMeta:  metav1.ObjectMeta{Namespace: namespace, Name: serviceName + "-slice", Labels: map[string]string{"kubernetes.io/service-name": serviceName}},
		AddressType: discoveryv1.AddressTypeIPv4,
		Endpoints: []discoveryv1.Endpoint{{
			Addresses:  []string{"10.0.0.10"},
			Conditions: discoveryv1.EndpointConditions{Ready: ready, Terminating: terminating},
		}},
	}
}
