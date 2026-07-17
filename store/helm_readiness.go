package store

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const helmPostInstallReadinessTimeout = helmInstallTimeout

func waitForHelmReleaseResources(parent context.Context, cfg *rest.Config, releaseName, namespace string) error {
	if parent == nil {
		parent = context.Background()
	}
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("create Helm readiness client: %w", err)
	}
	refs, err := helmReleaseResourceRefs(cfg, releaseName, namespace)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(parent, helmPostInstallReadinessTimeout)
	defer cancel()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	var lastErr error
	for {
		if err := validateHelmReleaseResourcesWithRefs(ctx, client, releaseName, namespace, refs); err == nil {
			return nil
		} else {
			lastErr = err
			if startupErr := helmPodStartupFailure(ctx, client, namespace, releaseName); startupErr != nil {
				return startupErr
			}
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

func helmPodStartupFailure(ctx context.Context, client kubernetes.Interface, namespace, releaseName string) error {
	selector := labels.Set{"app.kubernetes.io/instance": releaseName}.AsSelector().String()
	pods, err := client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return fmt.Errorf("list Helm release Pods for startup diagnostics: %w", err)
	}
	for _, pod := range pods.Items {
		statuses := append(append([]corev1.ContainerStatus{}, pod.Status.InitContainerStatuses...), pod.Status.ContainerStatuses...)
		for _, status := range statuses {
			if status.State.Waiting != nil && isHelmImageStartupFailure(status.State.Waiting.Reason) {
				return fmt.Errorf(
					"Helm release %s/%s Pod %s/%s container %s cannot start image %q: %s: %s",
					namespace,
					releaseName,
					pod.Namespace,
					pod.Name,
					status.Name,
					podImage(pod, status.Name),
					status.State.Waiting.Reason,
					strings.TrimSpace(status.State.Waiting.Message),
				)
			}
			if status.State.Terminated != nil && isHelmArchitectureFailure(status.State.Terminated) {
				return fmt.Errorf(
					"Helm release %s/%s Pod %s/%s container %s cannot start image %q: %s: %s",
					namespace,
					releaseName,
					pod.Namespace,
					pod.Name,
					status.Name,
					podImage(pod, status.Name),
					emptyHelmStatusReason(status.State.Terminated.Reason, "container cannot run"),
					strings.TrimSpace(status.State.Terminated.Message),
				)
			}
		}
	}
	return nil
}

func isHelmImageStartupFailure(reason string) bool {
	switch reason {
	case "InvalidImageName", "CreateContainerConfigError", "CreateContainerError":
		return true
	default:
		return false
	}
}

func isHelmArchitectureFailure(status *corev1.ContainerStateTerminated) bool {
	if status == nil {
		return false
	}
	message := strings.ToLower(status.Message + " " + status.Reason)
	return strings.Contains(message, "exec format error") || status.Reason == "ContainerCannotRun"
}

func podImage(pod corev1.Pod, containerName string) string {
	for _, container := range append(append([]corev1.Container{}, pod.Spec.InitContainers...), pod.Spec.Containers...) {
		if container.Name == containerName {
			return container.Image
		}
	}
	return "<unknown>"
}

func emptyHelmStatusReason(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

type helmResourceRef struct {
	kind      string
	name      string
	namespace string
}

var helmReadinessResourceKinds = map[string]struct{}{
	"DaemonSet":             {},
	"Deployment":            {},
	"Ingress":               {},
	"Job":                   {},
	"Pod":                   {},
	"PersistentVolumeClaim": {},
	"Service":               {},
	"StatefulSet":           {},
}

func helmReleaseResourceRefs(cfg *rest.Config, releaseName, namespace string) ([]helmResourceRef, error) {
	actionConfig, err := newHelmConfig(cfg, namespace)
	if err != nil {
		return nil, fmt.Errorf("create Helm release store: %w", err)
	}
	release, err := actionConfig.Releases.Last(releaseName)
	if err != nil {
		return nil, fmt.Errorf("read Helm release manifest: %w", err)
	}
	decoder := utilyaml.NewYAMLOrJSONDecoder(strings.NewReader(release.Manifest), 4096)
	refs := make([]helmResourceRef, 0)
	seen := make(map[string]struct{})
	for {
		var raw map[string]interface{}
		if err := decoder.Decode(&raw); err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("decode Helm release manifest: %w", err)
		}
		if len(raw) == 0 {
			continue
		}
		object := unstructured.Unstructured{Object: raw}
		if _, ok := helmReadinessResourceKinds[object.GetKind()]; !ok || object.GetName() == "" {
			continue
		}
		if object.GetAnnotations()["helm.sh/hook"] != "" {
			continue
		}
		refNamespace := object.GetNamespace()
		if refNamespace == "" {
			refNamespace = namespace
		}
		ref := helmResourceRef{kind: object.GetKind(), name: object.GetName(), namespace: refNamespace}
		key := strings.Join([]string{ref.kind, ref.namespace, ref.name}, "/")
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		refs = append(refs, ref)
	}
	return refs, nil
}

func validateHelmReleaseResources(ctx context.Context, client kubernetes.Interface, releaseName, namespace string) error {
	return validateHelmReleaseResourcesWithRefs(ctx, client, releaseName, namespace, nil)
}

func validateHelmReleaseResourcesWithRefs(ctx context.Context, client kubernetes.Interface, releaseName, namespace string, refs []helmResourceRef) error {
	selector := labels.Set{"app.kubernetes.io/instance": releaseName}.AsSelector().String()
	problems := make([]string, 0)
	appendProblem := func(problem string) {
		problems = append(problems, problem)
	}

	pods, err := client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return fmt.Errorf("list Helm release Pods: %w", err)
	}
	podKeys := make(map[string]struct{}, len(pods.Items))
	for _, pod := range pods.Items {
		podKeys[resourceKey(pod.Namespace, pod.Name)] = struct{}{}
		if !podReadyForHelm(pod) {
			appendProblem(fmt.Sprintf("Pod %s/%s is not ready", pod.Namespace, pod.Name))
		}
	}
	for _, ref := range refsForKind(refs, "Pod") {
		if _, ok := podKeys[resourceKey(ref.namespace, ref.name)]; ok {
			continue
		}
		pod, err := client.CoreV1().Pods(ref.namespace).Get(ctx, ref.name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			appendProblem(fmt.Sprintf("Pod %s/%s is missing", ref.namespace, ref.name))
			continue
		}
		if err != nil {
			return fmt.Errorf("get Helm release Pod %s/%s: %w", ref.namespace, ref.name, err)
		}
		if !podReadyForHelm(*pod) {
			appendProblem(fmt.Sprintf("Pod %s/%s is not ready", ref.namespace, ref.name))
		}
	}

	deployments, err := client.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return fmt.Errorf("list Helm release Deployments: %w", err)
	}
	deploymentKeys := make(map[string]struct{}, len(deployments.Items))
	for _, deployment := range deployments.Items {
		deploymentKeys[resourceKey(deployment.Namespace, deployment.Name)] = struct{}{}
		desired := int32(1)
		if deployment.Spec.Replicas != nil {
			desired = *deployment.Spec.Replicas
		}
		if deployment.Status.AvailableReplicas < desired {
			appendProblem(fmt.Sprintf("Deployment %s/%s available %d/%d", namespace, deployment.Name, deployment.Status.AvailableReplicas, desired))
		}
	}
	for _, ref := range refsForKind(refs, "Deployment") {
		if _, ok := deploymentKeys[resourceKey(ref.namespace, ref.name)]; ok {
			continue
		}
		deployment, err := client.AppsV1().Deployments(ref.namespace).Get(ctx, ref.name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			appendProblem(fmt.Sprintf("Deployment %s/%s is missing", ref.namespace, ref.name))
			continue
		}
		if err != nil {
			return fmt.Errorf("get Helm release Deployment %s/%s: %w", ref.namespace, ref.name, err)
		}
		desired := int32(1)
		if deployment.Spec.Replicas != nil {
			desired = *deployment.Spec.Replicas
		}
		if deployment.Status.AvailableReplicas < desired {
			appendProblem(fmt.Sprintf("Deployment %s/%s available %d/%d", ref.namespace, ref.name, deployment.Status.AvailableReplicas, desired))
		}
	}

	statefulSets, err := client.AppsV1().StatefulSets(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return fmt.Errorf("list Helm release StatefulSets: %w", err)
	}
	statefulSetKeys := make(map[string]struct{}, len(statefulSets.Items))
	for _, statefulSet := range statefulSets.Items {
		statefulSetKeys[resourceKey(statefulSet.Namespace, statefulSet.Name)] = struct{}{}
		desired := int32(1)
		if statefulSet.Spec.Replicas != nil {
			desired = *statefulSet.Spec.Replicas
		}
		if statefulSet.Status.ReadyReplicas < desired {
			appendProblem(fmt.Sprintf("StatefulSet %s/%s ready %d/%d", namespace, statefulSet.Name, statefulSet.Status.ReadyReplicas, desired))
		}
	}
	for _, ref := range refsForKind(refs, "StatefulSet") {
		if _, ok := statefulSetKeys[resourceKey(ref.namespace, ref.name)]; ok {
			continue
		}
		statefulSet, err := client.AppsV1().StatefulSets(ref.namespace).Get(ctx, ref.name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			appendProblem(fmt.Sprintf("StatefulSet %s/%s is missing", ref.namespace, ref.name))
			continue
		}
		if err != nil {
			return fmt.Errorf("get Helm release StatefulSet %s/%s: %w", ref.namespace, ref.name, err)
		}
		desired := int32(1)
		if statefulSet.Spec.Replicas != nil {
			desired = *statefulSet.Spec.Replicas
		}
		if statefulSet.Status.ReadyReplicas < desired {
			appendProblem(fmt.Sprintf("StatefulSet %s/%s ready %d/%d", ref.namespace, ref.name, statefulSet.Status.ReadyReplicas, desired))
		}
	}

	daemonSets, err := client.AppsV1().DaemonSets(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return fmt.Errorf("list Helm release DaemonSets: %w", err)
	}
	daemonSetKeys := make(map[string]struct{}, len(daemonSets.Items))
	for _, daemonSet := range daemonSets.Items {
		daemonSetKeys[resourceKey(daemonSet.Namespace, daemonSet.Name)] = struct{}{}
		if daemonSet.Status.NumberReady < daemonSet.Status.DesiredNumberScheduled {
			appendProblem(fmt.Sprintf("DaemonSet %s/%s ready %d/%d", namespace, daemonSet.Name, daemonSet.Status.NumberReady, daemonSet.Status.DesiredNumberScheduled))
		}
	}
	for _, ref := range refsForKind(refs, "DaemonSet") {
		if _, ok := daemonSetKeys[resourceKey(ref.namespace, ref.name)]; ok {
			continue
		}
		daemonSet, err := client.AppsV1().DaemonSets(ref.namespace).Get(ctx, ref.name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			appendProblem(fmt.Sprintf("DaemonSet %s/%s is missing", ref.namespace, ref.name))
			continue
		}
		if err != nil {
			return fmt.Errorf("get Helm release DaemonSet %s/%s: %w", ref.namespace, ref.name, err)
		}
		if daemonSet.Status.NumberReady < daemonSet.Status.DesiredNumberScheduled {
			appendProblem(fmt.Sprintf("DaemonSet %s/%s ready %d/%d", ref.namespace, ref.name, daemonSet.Status.NumberReady, daemonSet.Status.DesiredNumberScheduled))
		}
	}

	jobs, err := client.BatchV1().Jobs(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return fmt.Errorf("list Helm release Jobs: %w", err)
	}
	jobKeys := make(map[string]struct{}, len(jobs.Items))
	for _, job := range jobs.Items {
		jobKeys[resourceKey(job.Namespace, job.Name)] = struct{}{}
		checkJobReadiness(&job, appendProblem)
	}
	for _, ref := range refsForKind(refs, "Job") {
		if _, ok := jobKeys[resourceKey(ref.namespace, ref.name)]; ok {
			continue
		}
		job, err := client.BatchV1().Jobs(ref.namespace).Get(ctx, ref.name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			appendProblem(fmt.Sprintf("Job %s/%s is missing", ref.namespace, ref.name))
			continue
		}
		if err != nil {
			return fmt.Errorf("get Helm release Job %s/%s: %w", ref.namespace, ref.name, err)
		}
		checkJobReadiness(job, appendProblem)
	}

	pvcs, err := client.CoreV1().PersistentVolumeClaims(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return fmt.Errorf("list Helm release PVCs: %w", err)
	}
	pvcKeys := make(map[string]struct{}, len(pvcs.Items))
	for _, pvc := range pvcs.Items {
		pvcKeys[resourceKey(pvc.Namespace, pvc.Name)] = struct{}{}
		if pvc.Status.Phase != corev1.ClaimBound {
			appendProblem(fmt.Sprintf("PVC %s/%s is %s", namespace, pvc.Name, pvc.Status.Phase))
		}
	}
	for _, ref := range refsForKind(refs, "PersistentVolumeClaim") {
		if _, ok := pvcKeys[resourceKey(ref.namespace, ref.name)]; ok {
			continue
		}
		pvc, err := client.CoreV1().PersistentVolumeClaims(ref.namespace).Get(ctx, ref.name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			appendProblem(fmt.Sprintf("PVC %s/%s is missing", ref.namespace, ref.name))
			continue
		}
		if err != nil {
			return fmt.Errorf("get Helm release PVC %s/%s: %w", ref.namespace, ref.name, err)
		}
		if pvc.Status.Phase != corev1.ClaimBound {
			appendProblem(fmt.Sprintf("PVC %s/%s is %s", ref.namespace, ref.name, pvc.Status.Phase))
		}
	}

	services, err := client.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return fmt.Errorf("list Helm release Services: %w", err)
	}
	serviceKeys := make(map[string]struct{}, len(services.Items))
	for _, service := range services.Items {
		serviceKeys[resourceKey(service.Namespace, service.Name)] = struct{}{}
		if !serviceRequiresReadyEndpoint(service) {
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
	for _, ref := range refsForKind(refs, "Service") {
		if _, ok := serviceKeys[resourceKey(ref.namespace, ref.name)]; ok {
			continue
		}
		service, err := client.CoreV1().Services(ref.namespace).Get(ctx, ref.name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			appendProblem(fmt.Sprintf("Service %s/%s is missing", ref.namespace, ref.name))
			continue
		}
		if err != nil {
			return fmt.Errorf("get Helm release Service %s/%s: %w", ref.namespace, ref.name, err)
		}
		if !serviceRequiresReadyEndpoint(*service) {
			continue
		}
		ready, err := serviceHasReadyEndpointSlice(ctx, client, *service)
		if err != nil {
			return fmt.Errorf("check Service %s/%s endpoints: %w", ref.namespace, ref.name, err)
		}
		if !ready {
			appendProblem(fmt.Sprintf("Service %s/%s has no ready EndpointSlice", ref.namespace, ref.name))
		}
	}

	ingresses, err := client.NetworkingV1().Ingresses(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return fmt.Errorf("list Helm release Ingresses: %w", err)
	}
	ingressKeys := make(map[string]struct{}, len(ingresses.Items))
	for _, ingress := range ingresses.Items {
		ingressKeys[resourceKey(ingress.Namespace, ingress.Name)] = struct{}{}
		if err := checkIngressReadiness(ctx, client, &ingress, appendProblem); err != nil {
			return err
		}
	}
	for _, ref := range refsForKind(refs, "Ingress") {
		if _, ok := ingressKeys[resourceKey(ref.namespace, ref.name)]; ok {
			continue
		}
		ingress, err := client.NetworkingV1().Ingresses(ref.namespace).Get(ctx, ref.name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			appendProblem(fmt.Sprintf("Ingress %s/%s is missing", ref.namespace, ref.name))
			continue
		}
		if err != nil {
			return fmt.Errorf("get Helm release Ingress %s/%s: %w", ref.namespace, ref.name, err)
		}
		if err := checkIngressReadiness(ctx, client, ingress, appendProblem); err != nil {
			return err
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

func checkDeploymentReadiness(deployment *appsv1.Deployment, appendProblem func(string)) {
	desired := int32(1)
	if deployment.Spec.Replicas != nil {
		desired = *deployment.Spec.Replicas
	}
	if deployment.Status.AvailableReplicas < desired {
		appendProblem(fmt.Sprintf("Deployment %s/%s available %d/%d", deployment.Namespace, deployment.Name, deployment.Status.AvailableReplicas, desired))
	}
}

func checkStatefulSetReadiness(statefulSet *appsv1.StatefulSet, appendProblem func(string)) {
	desired := int32(1)
	if statefulSet.Spec.Replicas != nil {
		desired = *statefulSet.Spec.Replicas
	}
	if statefulSet.Status.ReadyReplicas < desired {
		appendProblem(fmt.Sprintf("StatefulSet %s/%s ready %d/%d", statefulSet.Namespace, statefulSet.Name, statefulSet.Status.ReadyReplicas, desired))
	}
}

func checkDaemonSetReadiness(daemonSet *appsv1.DaemonSet, appendProblem func(string)) {
	if daemonSet.Status.NumberReady < daemonSet.Status.DesiredNumberScheduled {
		appendProblem(fmt.Sprintf("DaemonSet %s/%s ready %d/%d", daemonSet.Namespace, daemonSet.Name, daemonSet.Status.NumberReady, daemonSet.Status.DesiredNumberScheduled))
	}
}

func podReadyForHelm(pod corev1.Pod) bool {
	if pod.Status.Phase == corev1.PodSucceeded {
		return true
	}
	if pod.Status.Phase != corev1.PodRunning {
		return false
	}
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}

func checkJobReadiness(job *batchv1.Job, appendProblem func(string)) {
	for _, condition := range job.Status.Conditions {
		if condition.Type == batchv1.JobFailed && condition.Status == corev1.ConditionTrue {
			appendProblem(fmt.Sprintf("Job %s/%s failed: %s", job.Namespace, job.Name, strings.TrimSpace(condition.Message)))
			return
		}
	}
	desired := int32(1)
	if job.Spec.Completions != nil {
		desired = *job.Spec.Completions
	}
	if job.Status.Succeeded < desired {
		appendProblem(fmt.Sprintf("Job %s/%s completed %d/%d", job.Namespace, job.Name, job.Status.Succeeded, desired))
	}
}

func checkPVCReadiness(pvc *corev1.PersistentVolumeClaim, appendProblem func(string)) {
	if pvc.Status.Phase != corev1.ClaimBound {
		appendProblem(fmt.Sprintf("PVC %s/%s is %s", pvc.Namespace, pvc.Name, pvc.Status.Phase))
	}
}

func checkServiceReadiness(ctx context.Context, client kubernetes.Interface, service *corev1.Service, appendProblem func(string)) error {
	if !serviceRequiresReadyEndpoint(*service) {
		return nil
	}
	ready, err := serviceHasReadyEndpointSlice(ctx, client, *service)
	if err != nil {
		return fmt.Errorf("check Service %s/%s endpoints: %w", service.Namespace, service.Name, err)
	}
	if !ready {
		appendProblem(fmt.Sprintf("Service %s/%s has no ready EndpointSlice", service.Namespace, service.Name))
	}
	return nil
}

func serviceRequiresReadyEndpoint(service corev1.Service) bool {
	return (service.Spec.Type == corev1.ServiceTypeNodePort || service.Spec.Type == corev1.ServiceTypeLoadBalancer) &&
		len(service.Spec.Selector) > 0
}

func checkIngressReadiness(ctx context.Context, client kubernetes.Interface, ingress *networkingv1.Ingress, appendProblem func(string)) error {
	if ingress.Spec.IngressClassName != nil && strings.TrimSpace(*ingress.Spec.IngressClassName) != "" {
		if _, err := client.NetworkingV1().IngressClasses().Get(ctx, *ingress.Spec.IngressClassName, metav1.GetOptions{}); apierrors.IsNotFound(err) {
			appendProblem(fmt.Sprintf("Ingress %s/%s references missing IngressClass %s", ingress.Namespace, ingress.Name, *ingress.Spec.IngressClassName))
		} else if err != nil {
			return fmt.Errorf("check IngressClass for %s/%s: %w", ingress.Namespace, ingress.Name, err)
		}
	}

	services := make(map[string]struct{})
	checkBackend := func(serviceName string) error {
		if serviceName == "" {
			return nil
		}
		if _, ok := services[serviceName]; ok {
			return nil
		}
		services[serviceName] = struct{}{}
		service, err := client.CoreV1().Services(ingress.Namespace).Get(ctx, serviceName, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			appendProblem(fmt.Sprintf("Ingress %s/%s backend Service %s/%s is missing", ingress.Namespace, ingress.Name, ingress.Namespace, serviceName))
			return nil
		}
		if err != nil {
			return fmt.Errorf("get Ingress backend Service %s/%s: %w", ingress.Namespace, serviceName, err)
		}
		if service.Spec.Type == corev1.ServiceTypeExternalName {
			return nil
		}
		ready, err := serviceHasReadyEndpointSlice(ctx, client, *service)
		if err != nil {
			return fmt.Errorf("check Ingress backend Service %s/%s endpoints: %w", ingress.Namespace, serviceName, err)
		}
		if !ready {
			appendProblem(fmt.Sprintf("Ingress %s/%s backend Service %s/%s has no ready EndpointSlice", ingress.Namespace, ingress.Name, ingress.Namespace, serviceName))
		}
		return nil
	}
	if ingress.Spec.DefaultBackend != nil && ingress.Spec.DefaultBackend.Service != nil {
		if err := checkBackend(ingress.Spec.DefaultBackend.Service.Name); err != nil {
			return err
		}
	}
	for _, rule := range ingress.Spec.Rules {
		if rule.HTTP == nil {
			continue
		}
		for _, path := range rule.HTTP.Paths {
			if path.Backend.Service == nil {
				continue
			}
			if err := checkBackend(path.Backend.Service.Name); err != nil {
				return err
			}
		}
	}
	return nil
}

func refsForKind(refs []helmResourceRef, kind string) []helmResourceRef {
	result := make([]helmResourceRef, 0)
	for _, ref := range refs {
		if ref.kind == kind {
			result = append(result, ref)
		}
	}
	return result
}

func resourceKey(namespace, name string) string {
	return namespace + "/" + name
}
