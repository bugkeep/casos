package server

import (
	"context"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestCreateOrUpdateDeploymentDoesNotMutateDesiredObject(t *testing.T) {
	desired := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "default"},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "demo"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "demo"}},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "demo", Image: "demo:1"}}},
			},
		},
	}
	original := desired.DeepCopy()
	if err := createOrUpdateDeployment(context.Background(), fake.NewSimpleClientset(), desired); err != nil {
		t.Fatalf("create deployment: %v", err)
	}
	if !apiequality.Semantic.DeepEqual(desired, original) {
		t.Fatalf("desired deployment was mutated\nwant: %#v\n got: %#v", original.Spec, desired.Spec)
	}
}

func TestWaitForServiceDeletedWaitsUntilObjectDisappears(t *testing.T) {
	service := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "kube-dns", Namespace: "kube-system"}}
	client := fake.NewSimpleClientset(service)
	go func() {
		time.Sleep(25 * time.Millisecond)
		_ = client.Tracker().Delete(corev1.SchemeGroupVersion.WithResource("services"), "kube-system", "kube-dns")
	}()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := waitForServiceDeleted(ctx, client, "kube-system", "kube-dns"); err != nil {
		t.Fatalf("wait for service deletion: %v", err)
	}
}
