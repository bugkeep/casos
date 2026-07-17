package store

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

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

const (
	helmPostInstallReadinessTimeout = helmOperationTimeout
	helmReadinessInitialPoll        = 2 * time.Second
	helmReadinessMaxPoll            = 10 * time.Second
	helmManifestDecoderBuffer       = 4096
)

// waitForHelmReleaseResources complements Helm's native Wait/WaitForJobs
// implementation. Helm owns workload, Pod, Job and PVC readiness; CasOS only
// waits for Service endpoints and Ingress backends, which Helm does not prove
// are usable.
func waitForHelmReleaseResources(parent context.Context, cfg *rest.Config, releaseName, namespace string) error {
	if cfg == nil {
		return errors.New("Helm readiness REST config is nil")
	}
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
	if len(refs) == 0 {
		return nil
	}

	ctx, cancel := context.WithTimeout(parent, helmPostInstallReadinessTimeout)
	defer cancel()
	pollInterval := helmReadinessInitialPoll
	var lastErr error
	for {
		lastErr = validateHelmReleaseResourcesWithRefs(ctx, client, releaseName, namespace, refs)
		if lastErr == nil {
			return nil
		}
		timer := time.NewTimer(pollInterval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return fmt.Errorf("Helm release %s/%s endpoint readiness stopped: %w (last check: %v)", namespace, releaseName, ctx.Err(), lastErr)
		case <-timer.C:
			if pollInterval < helmReadinessMaxPoll {
				pollInterval *= 2
				if pollInterval > helmReadinessMaxPoll {
					pollInterval = helmReadinessMaxPoll
				}
			}
		}
	}
}

type helmResourceRef struct {
	kind      string
	name      string
	namespace string
}

var helmReadinessResourceKinds = map[string]struct{}{
	"Ingress": {},
	"Service": {},
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
	decoder := utilyaml.NewYAMLOrJSONDecoder(strings.NewReader(release.Manifest), helmManifestDecoderBuffer)
	refs := make([]helmResourceRef, 0)
	seen := make(map[string]struct{})
	for {
		var raw map[string]interface{}
		if err := decoder.Decode(&raw); err != nil {
			if errors.Is(err, io.EOF) {
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

func validateHelmReleaseResourcesWithRefs(ctx context.Context, client kubernetes.Interface, releaseName, namespace string, refs []helmResourceRef) error {
	if client == nil {
		return errors.New("Helm readiness Kubernetes client is nil")
	}
	problems := make([]string, 0)
	appendProblem := func(problem string) {
		problems = append(problems, problem)
	}

	for _, ref := range refsForKind(refs, "Service") {
		service, err := client.CoreV1().Services(ref.namespace).Get(ctx, ref.name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			appendProblem(fmt.Sprintf("Service %s/%s is missing", ref.namespace, ref.name))
			continue
		}
		if err != nil {
			return fmt.Errorf("get Helm release Service %s/%s: %w", ref.namespace, ref.name, err)
		}
		if err := checkServiceReadiness(ctx, client, service, appendProblem); err != nil {
			return err
		}
	}

	for _, ref := range refsForKind(refs, "Ingress") {
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
	if apierrors.IsNotFound(err) {
		return serviceHasReadyEndpoints(ctx, client, service)
	}
	if err != nil {
		return false, err
	}
	for _, endpointSlice := range slices.Items {
		for _, endpoint := range endpointSlice.Endpoints {
			// Kubernetes defines nil Ready as true for backward compatibility.
			ready := endpoint.Conditions.Ready == nil || *endpoint.Conditions.Ready
			terminating := endpoint.Conditions.Terminating != nil && *endpoint.Conditions.Terminating
			if ready && !terminating {
				return true, nil
			}
		}
	}
	return false, nil
}

func serviceHasReadyEndpoints(ctx context.Context, client kubernetes.Interface, service corev1.Service) (bool, error) {
	endpoints, err := client.CoreV1().Endpoints(service.Namespace).Get(ctx, service.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	for _, subset := range endpoints.Subsets {
		if len(subset.Addresses) > 0 {
			return true, nil
		}
	}
	return false, nil
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
	return service.Spec.Type != corev1.ServiceTypeExternalName
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
		if !serviceRequiresReadyEndpoint(*service) {
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
				// Resource backends are controller-specific and have no generic
				// Kubernetes readiness signal for CasOS to inspect.
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
