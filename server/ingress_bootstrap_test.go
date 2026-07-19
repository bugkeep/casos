package server

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestEnsureIngressClassDoesNotCreateSecondDefault(t *testing.T) {
	client := fake.NewSimpleClientset(&networkingv1.IngressClass{
		ObjectMeta: metav1.ObjectMeta{Name: "nginx", Annotations: map[string]string{
			"ingressclass.kubernetes.io/is-default-class": "true",
		}},
		Spec: networkingv1.IngressClassSpec{Controller: "k8s.io/ingress-nginx"},
	})
	if err := ensureIngressClass(context.Background(), client); err != nil {
		t.Fatalf("ensure ingress class: %v", err)
	}
	created, err := client.NetworkingV1().IngressClasses().Get(context.Background(), ingressControllerClass, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get created ingress class: %v", err)
	}
	if created.Annotations["ingressclass.kubernetes.io/is-default-class"] == "true" {
		t.Fatal("CasOS ingress class became default even though another default already exists")
	}
}

func TestEnsureIngressClassYieldsDefaultToExistingController(t *testing.T) {
	client := fake.NewSimpleClientset(
		&networkingv1.IngressClass{ObjectMeta: metav1.ObjectMeta{
			Name:        ingressControllerClass,
			Labels:      ingressControllerLabels(),
			Annotations: map[string]string{"ingressclass.kubernetes.io/is-default-class": "true"},
		}, Spec: networkingv1.IngressClassSpec{Controller: ingressControllerID}},
		&networkingv1.IngressClass{ObjectMeta: metav1.ObjectMeta{
			Name: "nginx", Annotations: map[string]string{"ingressclass.kubernetes.io/is-default-class": "true"},
		}, Spec: networkingv1.IngressClassSpec{Controller: "k8s.io/ingress-nginx"}},
	)
	if err := ensureIngressClass(context.Background(), client); err != nil {
		t.Fatalf("ensure ingress class: %v", err)
	}
	stored, _ := client.NetworkingV1().IngressClasses().Get(context.Background(), ingressControllerClass, metav1.GetOptions{})
	if stored.Annotations["ingressclass.kubernetes.io/is-default-class"] == "true" {
		t.Fatal("CasOS ingress class remained default after another default appeared")
	}
}

func TestEnsureIngressControllerRefusesUnmanagedDeploymentBeforeMutation(t *testing.T) {
	deployment := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{
		Name: ingressControllerName, Namespace: ingressControllerNamespace,
		Labels: map[string]string{"app.kubernetes.io/managed-by": "helm"},
	}}
	client := fake.NewSimpleClientset(deployment)
	if err := ensureIngressController(context.Background(), client, Config{}); err == nil {
		t.Fatal("expected an unmanaged Deployment collision to be rejected")
	}
	stored, _ := client.AppsV1().Deployments(ingressControllerNamespace).Get(context.Background(), ingressControllerName, metav1.GetOptions{})
	if stored.Labels["app.kubernetes.io/managed-by"] != "helm" {
		t.Fatalf("unmanaged Deployment was mutated: %v", stored.Labels)
	}
}

func TestEnsureIngressClassDoesNotTakeOverUnmanagedClass(t *testing.T) {
	client := fake.NewSimpleClientset(&networkingv1.IngressClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:        ingressControllerClass,
			Labels:      map[string]string{"app.kubernetes.io/managed-by": "helm"},
			Annotations: map[string]string{"ingressclass.kubernetes.io/is-default-class": "false"},
		},
		Spec: networkingv1.IngressClassSpec{Controller: "traefik.io/ingress-controller"},
	})
	if err := ensureIngressClass(context.Background(), client); err == nil {
		t.Fatal("expected an unmanaged IngressClass collision to be rejected")
	}
	stored, _ := client.NetworkingV1().IngressClasses().Get(context.Background(), ingressControllerClass, metav1.GetOptions{})
	if stored.Labels["app.kubernetes.io/managed-by"] != "helm" || stored.Annotations["ingressclass.kubernetes.io/is-default-class"] != "false" {
		t.Fatalf("unmanaged ingress class was taken over: labels=%v annotations=%v", stored.Labels, stored.Annotations)
	}
}

func TestIngressControllerDeploymentUsesRestrictedSecurityContext(t *testing.T) {
	container := buildIngressControllerDeployment(Config{}).Spec.Template.Spec.Containers[0]
	if container.SecurityContext == nil || container.SecurityContext.AllowPrivilegeEscalation == nil || *container.SecurityContext.AllowPrivilegeEscalation {
		t.Fatal("expected privilege escalation to be disabled")
	}
	if container.SecurityContext.ReadOnlyRootFilesystem == nil || !*container.SecurityContext.ReadOnlyRootFilesystem {
		t.Fatal("expected a read-only root filesystem")
	}
	if container.LivenessProbe == nil {
		t.Fatal("expected a liveness probe")
	}
	if container.Resources.Requests.Cpu().IsZero() || container.Resources.Requests.Memory().IsZero() {
		t.Fatal("expected CPU and memory requests")
	}
	podContext := buildIngressControllerDeployment(Config{}).Spec.Template.Spec.SecurityContext
	if podContext == nil || podContext.RunAsNonRoot == nil || !*podContext.RunAsNonRoot {
		t.Fatal("expected ingress controller to run as non-root")
	}
}

func TestEnsureIngressControllerServiceRefusesUnmanagedService(t *testing.T) {
	client := fake.NewSimpleClientset(&corev1.Service{ObjectMeta: metav1.ObjectMeta{
		Name: ingressControllerName, Namespace: ingressControllerNamespace,
	}})
	if err := ensureIngressControllerService(context.Background(), client); err == nil {
		t.Fatal("expected an unmanaged Service collision to be rejected")
	}
}

func TestEnsureIngressControllerDeploymentRefusesUnmanagedDeploymentAtMutation(t *testing.T) {
	deployment := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{
		Name: ingressControllerName, Namespace: ingressControllerNamespace,
		Labels: map[string]string{"app.kubernetes.io/managed-by": "helm"},
	}}
	client := fake.NewSimpleClientset(deployment)
	if err := ensureIngressControllerDeployment(context.Background(), client, Config{}); err == nil {
		t.Fatal("expected the mutating path to reject an unmanaged Deployment")
	}
	stored, _ := client.AppsV1().Deployments(ingressControllerNamespace).Get(context.Background(), ingressControllerName, metav1.GetOptions{})
	if stored.Labels["app.kubernetes.io/managed-by"] != "helm" {
		t.Fatalf("unmanaged Deployment was adopted: %v", stored.Labels)
	}
}

func TestEnsureIngressControllerClusterRoleRefusesUnmanagedRoleAtMutation(t *testing.T) {
	role := &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{
		Name: "casos:traefik-ingress-controller", Labels: map[string]string{"app.kubernetes.io/managed-by": "helm"},
	}}
	client := fake.NewSimpleClientset(role)
	if err := ensureIngressControllerClusterRole(context.Background(), client); err == nil {
		t.Fatal("expected the mutating path to reject an unmanaged ClusterRole")
	}
	stored, _ := client.RbacV1().ClusterRoles().Get(context.Background(), role.Name, metav1.GetOptions{})
	if stored.Labels["app.kubernetes.io/managed-by"] != "helm" {
		t.Fatalf("unmanaged ClusterRole was adopted: %v", stored.Labels)
	}
}

func TestCleanupIngressControllerDeletesOnlyManagedResources(t *testing.T) {
	managed := ingressControllerLabels()
	client := fake.NewSimpleClientset(
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: ingressControllerName, Namespace: ingressControllerNamespace, Labels: managed}},
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: ingressControllerName, Namespace: ingressControllerNamespace, Labels: map[string]string{"app.kubernetes.io/managed-by": "helm"}}},
	)
	if err := cleanupIngressController(context.Background(), client); err != nil {
		t.Fatalf("cleanup ingress controller: %v", err)
	}
	if _, err := client.AppsV1().Deployments(ingressControllerNamespace).Get(context.Background(), ingressControllerName, metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("managed Deployment was not deleted: %v", err)
	}
	if _, err := client.CoreV1().Services(ingressControllerNamespace).Get(context.Background(), ingressControllerName, metav1.GetOptions{}); err != nil {
		t.Fatalf("foreign Service was deleted: %v", err)
	}
}
