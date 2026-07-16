package store

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const helmPostInstallReadinessTimeout = 2 * time.Minute

func waitForHelmReleaseResources(parent context.Context, cfg *rest.Config, releaseName, namespace string) error {
	if parent == nil {
		parent = context.Background()
	}
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("create Helm readiness client: %w", err)
	}
	ctx, cancel := context.WithTimeout(parent, helmPostInstallReadinessTimeout)
	defer cancel()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	var lastErr error
	for {
		if err := validateHelmReleaseResources(ctx, client, releaseName, namespace); err == nil {
			return nil
		} else {
			lastErr = err
		}
		select {
		case <-ctx.Done():
			if lastErr != nil {
				return lastErr
			}
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func validateHelmReleaseResources(ctx context.Context, client kubernetes.Interface, releaseName, namespace string) error {
	selector := labels.Set{"app.kubernetes.io/instance": releaseName}.AsSelector().String()
	problems := make([]string, 0)
	appendProblem := func(problem string) {
		problems = append(problems, problem)
	}

	deployments, err := client.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return fmt.Errorf("list Helm release Deployments: %w", err)
	}
	for _, deployment := range deployments.Items {
		desired := int32(1)
		if deployment.Spec.Replicas != nil {
			desired = *deployment.Spec.Replicas
		}
		if deployment.Status.AvailableReplicas < desired {
			appendProblem(fmt.Sprintf("Deployment %s/%s available %d/%d", namespace, deployment.Name, deployment.Status.AvailableReplicas, desired))
		}
	}

	statefulSets, err := client.AppsV1().StatefulSets(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return fmt.Errorf("list Helm release StatefulSets: %w", err)
	}
	for _, statefulSet := range statefulSets.Items {
		desired := int32(1)
		if statefulSet.Spec.Replicas != nil {
			desired = *statefulSet.Spec.Replicas
		}
		if statefulSet.Status.ReadyReplicas < desired {
			appendProblem(fmt.Sprintf("StatefulSet %s/%s ready %d/%d", namespace, statefulSet.Name, statefulSet.Status.ReadyReplicas, desired))
		}
	}

	daemonSets, err := client.AppsV1().DaemonSets(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return fmt.Errorf("list Helm release DaemonSets: %w", err)
	}
	for _, daemonSet := range daemonSets.Items {
		if daemonSet.Status.NumberReady < daemonSet.Status.DesiredNumberScheduled {
			appendProblem(fmt.Sprintf("DaemonSet %s/%s ready %d/%d", namespace, daemonSet.Name, daemonSet.Status.NumberReady, daemonSet.Status.DesiredNumberScheduled))
		}
	}

	jobs, err := client.BatchV1().Jobs(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return fmt.Errorf("list Helm release Jobs: %w", err)
	}
	for _, job := range jobs.Items {
		for _, condition := range job.Status.Conditions {
			if condition.Type == batchv1.JobFailed && condition.Status == corev1.ConditionTrue {
				appendProblem(fmt.Sprintf("Job %s/%s failed: %s", namespace, job.Name, strings.TrimSpace(condition.Message)))
				break
			}
		}
	}

	pvcs, err := client.CoreV1().PersistentVolumeClaims(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return fmt.Errorf("list Helm release PVCs: %w", err)
	}
	for _, pvc := range pvcs.Items {
		if pvc.Status.Phase != corev1.ClaimBound {
			appendProblem(fmt.Sprintf("PVC %s/%s is %s", namespace, pvc.Name, pvc.Status.Phase))
		}
	}

	services, err := client.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return fmt.Errorf("list Helm release Services: %w", err)
	}
	for _, service := range services.Items {
		if service.Spec.Type == corev1.ServiceTypeExternalName || service.Spec.ClusterIP == corev1.ClusterIPNone || len(service.Spec.Selector) == 0 {
			continue
		}
		ready, err := serviceHasReadyEndpointSlice(ctx, client, service)
		if err != nil {
			return fmt.Errorf("check Service %s/%s endpoints: %w", namespace, service.Name, err)
		}
		if !ready {
			appendProblem(fmt.Sprintf("Service %s/%s has no ready EndpointSlice", namespace, service.Name))
		}
	}

	ingresses, err := client.NetworkingV1().Ingresses(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return fmt.Errorf("list Helm release Ingresses: %w", err)
	}
	for _, ingress := range ingresses.Items {
		if ingress.Spec.IngressClassName == nil || strings.TrimSpace(*ingress.Spec.IngressClassName) == "" {
			continue
		}
		if _, err := client.NetworkingV1().IngressClasses().Get(ctx, *ingress.Spec.IngressClassName, metav1.GetOptions{}); apierrors.IsNotFound(err) {
			appendProblem(fmt.Sprintf("Ingress %s/%s references missing IngressClass %s", namespace, ingress.Name, *ingress.Spec.IngressClassName))
		} else if err != nil {
			return fmt.Errorf("check IngressClass for %s/%s: %w", namespace, ingress.Name, err)
		}
	}

	if len(problems) == 0 {
		return nil
	}
	sort.Strings(problems)
	return fmt.Errorf("Helm release %s/%s is not operational: %s", namespace, releaseName, strings.Join(problems, "; "))
}

func serviceHasReadyEndpointSlice(ctx context.Context, client kubernetes.Interface, service corev1.Service) (bool, error) {
	slices, err := client.DiscoveryV1().EndpointSlices(service.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labels.Set{"kubernetes.io/service-name": service.Name}.AsSelector().String(),
	})
	if err != nil {
		return false, err
	}
	for _, endpointSlice := range slices.Items {
		for _, endpoint := range endpointSlice.Endpoints {
			if endpoint.Conditions.Ready == nil || *endpoint.Conditions.Ready {
				return true, nil
			}
		}
	}
	return false, nil
}
