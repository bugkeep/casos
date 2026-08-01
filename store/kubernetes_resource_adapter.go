package store

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"

	"helm.sh/helm/v3/pkg/action"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/restmapper"
)

type kubernetesResourceErrorCode string

const (
	kubernetesInvalidManifest   kubernetesResourceErrorCode = "INVALID_MANIFEST"
	kubernetesResourceNotServed kubernetesResourceErrorCode = "RESOURCE_NOT_SERVED"
	kubernetesResourceNotFound  kubernetesResourceErrorCode = "RESOURCE_NOT_FOUND"
	kubernetesForbidden         kubernetesResourceErrorCode = "FORBIDDEN_BY_RBAC"
	kubernetesConflict          kubernetesResourceErrorCode = "CONFLICT"
	kubernetesDiscoveryFailed   kubernetesResourceErrorCode = "DISCOVERY_FAILED"
	kubernetesAPIUnavailable    kubernetesResourceErrorCode = "API_UNAVAILABLE"
	kubernetesHelmDryRunFailed  kubernetesResourceErrorCode = "HELM_DRY_RUN_FAILED"
)

type kubernetesResourceError struct {
	Code kubernetesResourceErrorCode
	GVK  schema.GroupVersionKind
	Err  error
}

func (e *kubernetesResourceError) Error() string {
	if e.GVK.Empty() {
		return fmt.Sprintf("%s: %v", e.Code, e.Err)
	}
	return fmt.Sprintf("%s for %s: %v", e.Code, e.GVK.String(), e.Err)
}

func (e *kubernetesResourceError) Unwrap() error {
	return e.Err
}

type resolvedKubernetesResource struct {
	GVK        schema.GroupVersionKind
	GVR        schema.GroupVersionResource
	Namespaced bool
	Namespace  string
	PendingCRD *pendingHelmCRD
}

type pendingHelmCRD struct {
	GVK schema.GroupVersionKind
	helmChartCRDDefinition
}

// kubernetesResourceAdapter resolves rendered Helm resources against the
// target cluster. Helm remains responsible for release dry-runs and writes.
type kubernetesResourceAdapter struct {
	mapper  meta.RESTMapper
	refresh func()
	getCRD  func(context.Context, string) (*unstructured.Unstructured, error)
}

func newKubernetesResourceAdapter(getter action.RESTClientGetter) (*kubernetesResourceAdapter, error) {
	if getter == nil {
		return nil, &kubernetesResourceError{
			Code: kubernetesAPIUnavailable,
			Err:  fmt.Errorf("Helm REST client getter is missing"),
		}
	}
	discoveryClient, err := getter.ToDiscoveryClient()
	if err != nil {
		return nil, classifyKubernetesResourceError(schema.GroupVersionKind{}, err)
	}
	restConfig, err := getter.ToRESTConfig()
	if err != nil {
		return nil, classifyKubernetesResourceError(schema.GroupVersionKind{}, err)
	}
	dynamicClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return nil, classifyKubernetesResourceError(schema.GroupVersionKind{}, err)
	}
	crds := dynamicClient.Resource(schema.GroupVersionResource{
		Group:    "apiextensions.k8s.io",
		Version:  "v1",
		Resource: "customresourcedefinitions",
	})
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(discoveryClient)
	return &kubernetesResourceAdapter{
		mapper:  mapper,
		refresh: mapper.Reset,
		getCRD: func(ctx context.Context, name string) (*unstructured.Unstructured, error) {
			return crds.Get(ctx, name, metav1.GetOptions{})
		},
	}, nil
}

func (a *kubernetesResourceAdapter) resolveManifest(ctx context.Context, manifest, defaultNamespace string, chartCRDs map[schema.GroupVersionKind]helmChartCRDDefinition) ([]resolvedKubernetesResource, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if a == nil || a.mapper == nil {
		return nil, &kubernetesResourceError{
			Code: kubernetesAPIUnavailable,
			Err:  fmt.Errorf("Kubernetes REST mapper is missing"),
		}
	}

	decoder := utilyaml.NewYAMLOrJSONDecoder(strings.NewReader(manifest), 4096)
	resources := make([]resolvedKubernetesResource, 0)
	for {
		if err := ctx.Err(); err != nil {
			return nil, classifyKubernetesResourceError(schema.GroupVersionKind{}, err)
		}
		var document map[string]interface{}
		if err := decoder.Decode(&document); err != nil {
			if err == io.EOF {
				break
			}
			return nil, &kubernetesResourceError{Code: kubernetesInvalidManifest, Err: fmt.Errorf("decode rendered Helm manifest: %w", err)}
		}
		if len(document) == 0 {
			continue
		}
		if err := a.resolveDocument(ctx, document, defaultNamespace, chartCRDs, &resources); err != nil {
			return nil, err
		}
	}
	return resources, nil
}

func (a *kubernetesResourceAdapter) resolveDocument(ctx context.Context, document map[string]interface{}, defaultNamespace string, chartCRDs map[schema.GroupVersionKind]helmChartCRDDefinition, resources *[]resolvedKubernetesResource) error {
	if err := ctx.Err(); err != nil {
		return classifyKubernetesResourceError(schema.GroupVersionKind{}, err)
	}
	kind, _, err := unstructured.NestedString(document, "kind")
	if err != nil {
		return &kubernetesResourceError{Code: kubernetesInvalidManifest, Err: fmt.Errorf("read kind: %w", err)}
	}
	kind = strings.TrimSpace(kind)
	if kind == "List" {
		items, _, err := unstructured.NestedSlice(document, "items")
		if err != nil {
			return &kubernetesResourceError{Code: kubernetesInvalidManifest, Err: fmt.Errorf("decode List items: %w", err)}
		}
		for _, item := range items {
			object, ok := item.(map[string]interface{})
			if !ok {
				return &kubernetesResourceError{Code: kubernetesInvalidManifest, Err: fmt.Errorf("List item is not a Kubernetes object")}
			}
			if err := a.resolveDocument(ctx, object, defaultNamespace, chartCRDs, resources); err != nil {
				return err
			}
		}
		return nil
	}

	apiVersion, _, err := unstructured.NestedString(document, "apiVersion")
	if err != nil {
		return &kubernetesResourceError{Code: kubernetesInvalidManifest, Err: fmt.Errorf("read apiVersion: %w", err)}
	}
	apiVersion = strings.TrimSpace(apiVersion)
	if apiVersion == "" || kind == "" {
		return &kubernetesResourceError{Code: kubernetesInvalidManifest, Err: fmt.Errorf("apiVersion and kind are required")}
	}
	groupVersion, err := schema.ParseGroupVersion(apiVersion)
	if err != nil {
		return &kubernetesResourceError{Code: kubernetesInvalidManifest, Err: fmt.Errorf("parse apiVersion %q: %w", apiVersion, err)}
	}
	gvk := groupVersion.WithKind(kind)

	mapping, err := a.mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if meta.IsNoMatchError(err) && a.refresh != nil {
		a.refresh()
		if err := ctx.Err(); err != nil {
			return classifyKubernetesResourceError(gvk, err)
		}
		mapping, err = a.mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	}
	var resource schema.GroupVersionResource
	var namespaced bool
	if err != nil {
		if meta.IsNoMatchError(err) {
			definition, declaredByChart := chartCRDs[gvk]
			if !declaredByChart {
				return &kubernetesResourceError{Code: kubernetesResourceNotServed, GVK: gvk, Err: err}
			}
			resolved, resolveErr := a.resolveChartCRD(ctx, gvk, definition)
			if resolveErr != nil {
				return resolveErr
			}
			if resolved.PendingCRD != nil {
				*resources = append(*resources, resolved)
				return nil
			}
			resource = resolved.GVR
			namespaced = resolved.Namespaced
		} else {
			return classifyKubernetesResourceError(gvk, err)
		}
	} else {
		resource = mapping.Resource
		namespaced = mapping.Scope.Name() == meta.RESTScopeNameNamespace
	}

	namespace := ""
	if namespaced {
		namespace, _, err = unstructured.NestedString(document, "metadata", "namespace")
		if err != nil {
			return &kubernetesResourceError{Code: kubernetesInvalidManifest, GVK: gvk, Err: fmt.Errorf("read metadata.namespace: %w", err)}
		}
		if strings.TrimSpace(namespace) == "" {
			namespace = defaultNamespace
		}
	}
	*resources = append(*resources, resolvedKubernetesResource{
		GVK:        gvk,
		GVR:        resource,
		Namespaced: namespaced,
		Namespace:  namespace,
	})
	return nil
}

func (a *kubernetesResourceAdapter) resolveChartCRD(ctx context.Context, gvk schema.GroupVersionKind, definition helmChartCRDDefinition) (resolvedKubernetesResource, error) {
	if a.getCRD == nil {
		return resolvedKubernetesResource{}, &kubernetesResourceError{
			Code: kubernetesAPIUnavailable,
			GVK:  gvk,
			Err:  fmt.Errorf("CustomResourceDefinition client is missing"),
		}
	}
	existing, err := a.getCRD(ctx, definition.Name)
	if apierrors.IsNotFound(err) {
		pending := pendingHelmCRD{GVK: gvk, helmChartCRDDefinition: definition}
		return resolvedKubernetesResource{GVK: gvk, PendingCRD: &pending}, nil
	}
	if err != nil {
		return resolvedKubernetesResource{}, classifyKubernetesResourceError(gvk, err)
	}
	return resolveExistingCRD(gvk, existing)
}

func resolveExistingCRD(gvk schema.GroupVersionKind, crd *unstructured.Unstructured) (resolvedKubernetesResource, error) {
	if crd == nil {
		return resolvedKubernetesResource{}, crdDoesNotServeError(gvk, "existing CustomResourceDefinition is empty")
	}
	group, _, err := unstructured.NestedString(crd.Object, "spec", "group")
	if err != nil || group != gvk.Group {
		return resolvedKubernetesResource{}, crdDoesNotServeError(gvk, "existing CustomResourceDefinition has group %q", group)
	}
	kind, _, err := unstructured.NestedString(crd.Object, "spec", "names", "kind")
	if err != nil || kind != gvk.Kind {
		return resolvedKubernetesResource{}, crdDoesNotServeError(gvk, "existing CustomResourceDefinition has kind %q", kind)
	}
	versions, _, err := unstructured.NestedSlice(crd.Object, "spec", "versions")
	if err != nil {
		return resolvedKubernetesResource{}, crdDoesNotServeError(gvk, "read existing CustomResourceDefinition versions: %v", err)
	}
	served := false
	for _, item := range versions {
		version, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := version["name"].(string)
		isServed, _ := version["served"].(bool)
		if name == gvk.Version && isServed {
			served = true
			break
		}
	}
	if !served {
		return resolvedKubernetesResource{}, crdDoesNotServeError(gvk, "existing CustomResourceDefinition does not serve version %s", gvk.Version)
	}
	plural, _, err := unstructured.NestedString(crd.Object, "spec", "names", "plural")
	if err != nil || strings.TrimSpace(plural) == "" {
		return resolvedKubernetesResource{}, crdDoesNotServeError(gvk, "existing CustomResourceDefinition has no plural resource name")
	}
	scope, _, err := unstructured.NestedString(crd.Object, "spec", "scope")
	if err != nil || (scope != "Namespaced" && scope != "Cluster") {
		return resolvedKubernetesResource{}, crdDoesNotServeError(gvk, "existing CustomResourceDefinition has unsupported scope %q", scope)
	}
	return resolvedKubernetesResource{
		GVK:        gvk,
		GVR:        gvk.GroupVersion().WithResource(plural),
		Namespaced: scope == "Namespaced",
	}, nil
}

func crdDoesNotServeError(gvk schema.GroupVersionKind, format string, args ...interface{}) error {
	return &kubernetesResourceError{
		Code: kubernetesResourceNotServed,
		GVK:  gvk,
		Err:  fmt.Errorf(format, args...),
	}
}

func classifyKubernetesResourceError(gvk schema.GroupVersionKind, err error) error {
	return classifyKubernetesErrorWithDefault(gvk, err, kubernetesAPIUnavailable)
}

func classifyHelmDryRunError(gvk schema.GroupVersionKind, err error) error {
	if err != nil && isUnregisteredKindDryRunError(err) {
		return &kubernetesResourceError{Code: kubernetesResourceNotServed, GVK: gvk, Err: err}
	}
	return classifyKubernetesErrorWithDefault(gvk, err, kubernetesHelmDryRunFailed)
}

func classifyKubernetesErrorWithDefault(gvk schema.GroupVersionKind, err error, defaultCode kubernetesResourceErrorCode) error {
	if err == nil {
		return nil
	}
	code := defaultCode
	switch {
	case meta.IsNoMatchError(err):
		code = kubernetesResourceNotServed
	case apierrors.IsForbidden(err), apierrors.IsUnauthorized(err):
		code = kubernetesForbidden
	case apierrors.IsAlreadyExists(err), apierrors.IsConflict(err):
		code = kubernetesConflict
	case apierrors.IsNotFound(err):
		code = kubernetesResourceNotFound
	case apierrors.IsTimeout(err), apierrors.IsServerTimeout(err), apierrors.IsServiceUnavailable(err), apierrors.IsInternalError(err), apierrors.IsTooManyRequests(err), errors.Is(err, io.EOF), errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		code = kubernetesAPIUnavailable
	default:
		var networkError net.Error
		if errors.As(err, &networkError) {
			code = kubernetesAPIUnavailable
		}
		var discoveryError *discovery.ErrGroupDiscoveryFailed
		if errors.As(err, &discoveryError) {
			code = kubernetesDiscoveryFailed
		}
	}
	return &kubernetesResourceError{Code: code, GVK: gvk, Err: err}
}
