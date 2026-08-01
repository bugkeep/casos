package store

import (
	"context"
	"fmt"
	"io"
	"strings"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func validateHelmChartCompatibility(ctx context.Context, cfg *rest.Config, actionConfig *action.Configuration, releaseName, namespace string, chartToInstall *chart.Chart, values map[string]interface{}) error {
	if chartToInstall == nil || chartToInstall.Metadata == nil {
		return fmt.Errorf("chart metadata is missing")
	}
	if !isInstallableHelmChartMetadata(chartToInstall.Metadata) {
		return fmt.Errorf("chart %s is a library chart and cannot be installed as an application", chartToInstall.Name())
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, helmCompatibilityTimeout)
	defer cancel()
	chartCRDs, err := helmChartCRDDefinitions(chartToInstall)
	if err != nil {
		return fmt.Errorf("read chart %s CRDs: %w", chartToInstall.Name(), err)
	}
	namespaceExists, err := helmNamespaceExists(ctx, cfg, namespace)
	if err != nil {
		return err
	}
	// Helm's dry-run validates rendering and resource structure against the
	// target cluster. Remote template lookups are only safe after the release
	// namespace exists; the real install remains the source of truth.
	dryRun := newHelmCompatibilityDryRun(actionConfig, releaseName, namespace, namespaceExists)
	rendered, err := dryRun.RunWithContext(ctx, chartToInstall, values)
	if err == nil {
		return validateSuccessfulHelmDryRun(ctx, actionConfig, rendered.Manifest, namespace, namespaceExists, chartCRDs)
	}
	// A server-side dry-run rejects charts that declare a CRD and a custom
	// resource of that CRD in the same release, because the new kind is not yet
	// registered when the dry-run runs. Only exact GVKs declared by this chart
	// may use the client-render fallback; external prerequisites must still fail.
	if namespaceExists && isUnregisteredKindDryRunError(err) {
		clientDryRun := newHelmCompatibilityDryRun(actionConfig, releaseName, namespace, false)
		clientRendered, clientErr := clientDryRun.RunWithContext(ctx, chartToInstall, values)
		if clientErr != nil {
			return fmt.Errorf("render chart %s with client dry-run: %w", chartToInstall.Name(), classifyHelmDryRunError(schema.GroupVersionKind{}, clientErr))
		}
		if validateErr := validateHelmManifestGVKs(ctx, actionConfig, clientRendered.Manifest, namespace, chartCRDs); validateErr != nil {
			return fmt.Errorf("chart %s is not compatible with the target cluster: %w", chartToInstall.Name(), validateErr)
		}
		return nil
	}
	return fmt.Errorf("render chart %s for compatibility check: %w", chartToInstall.Name(), classifyHelmDryRunError(schema.GroupVersionKind{}, err))
}

func validateHelmReleaseCompatibility(ctx context.Context, actionConfig *action.Configuration, releaseName, namespace string, chartToInstall *chart.Chart, values map[string]interface{}) error {
	if chartToInstall == nil || chartToInstall.Metadata == nil {
		return fmt.Errorf("chart metadata is missing")
	}
	if !isInstallableHelmChartMetadata(chartToInstall.Metadata) {
		return fmt.Errorf("chart %s is a library chart and cannot be installed as an application", chartToInstall.Name())
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, helmCompatibilityTimeout)
	defer cancel()

	rendered, err := runHelmUpgradeDryRun(ctx, actionConfig, releaseName, namespace, chartToInstall, values)
	if err == nil {
		return validateSuccessfulHelmDryRun(ctx, actionConfig, rendered, namespace, true, nil)
	}
	return fmt.Errorf("render Helm release %s for compatibility check: %w", releaseName, classifyHelmDryRunError(schema.GroupVersionKind{}, err))
}

func runHelmUpgradeDryRun(ctx context.Context, actionConfig *action.Configuration, releaseName, namespace string, chartToInstall *chart.Chart, values map[string]interface{}) (string, error) {
	dryRun := action.NewUpgrade(actionConfig)
	dryRun.Namespace = namespace
	dryRun.DryRun = true
	dryRun.DryRunOption = "server"
	dryRun.Wait = true
	dryRun.WaitForJobs = true
	dryRun.Timeout = helmCompatibilityTimeout
	rendered, err := dryRun.RunWithContext(ctx, releaseName, chartToInstall, values)
	if err != nil {
		return "", err
	}
	return rendered.Manifest, nil
}

// isUnregisteredKindDryRunError reports whether a dry-run failure is caused by a
// custom resource whose CRD is created by the same release. A server-side
// dry-run cannot recognize such a kind before the CRD is applied, but a real
// install/upgrade applies CRDs first, so this specific failure is not a genuine
// incompatibility and must not block the operation.
func isUnregisteredKindDryRunError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	markers := []string{
		"unable to recognize",
		"no matches for kind",
		"ensure crds are installed first",
	}
	for _, marker := range markers {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}

func validateHelmManifestGVKs(ctx context.Context, actionConfig *action.Configuration, manifest, namespace string, chartCRDs map[schema.GroupVersionKind]helmChartCRDDefinition) error {
	_, err := resolveHelmManifestGVKs(ctx, actionConfig, manifest, namespace, chartCRDs)
	return err
}

func validateSuccessfulHelmDryRun(ctx context.Context, actionConfig *action.Configuration, manifest, namespace string, serverSide bool, chartCRDs map[schema.GroupVersionKind]helmChartCRDDefinition) error {
	if serverSide {
		return nil
	}
	return validateHelmManifestGVKs(ctx, actionConfig, manifest, namespace, chartCRDs)
}

func resolveHelmManifestGVKs(ctx context.Context, actionConfig *action.Configuration, manifest, namespace string, chartCRDs map[schema.GroupVersionKind]helmChartCRDDefinition) ([]resolvedKubernetesResource, error) {
	if actionConfig == nil {
		return nil, &kubernetesResourceError{Code: kubernetesAPIUnavailable, Err: fmt.Errorf("Helm action configuration is missing")}
	}
	adapter, err := newKubernetesResourceAdapter(actionConfig.RESTClientGetter)
	if err != nil {
		return nil, err
	}
	return adapter.resolveManifest(ctx, manifest, namespace, chartCRDs)
}

type helmChartCRDDefinition struct {
	Name           string
	Group          string
	Kind           string
	ServedVersions []string
}

func helmChartCRDDefinitions(ch *chart.Chart) (map[schema.GroupVersionKind]helmChartCRDDefinition, error) {
	definitions := make(map[schema.GroupVersionKind]helmChartCRDDefinition)
	if ch == nil {
		return definitions, nil
	}
	for _, crd := range ch.CRDObjects() {
		if crd.File == nil {
			return nil, fmt.Errorf("CRD file %s is missing", crd.Name)
		}
		decoder := utilyaml.NewYAMLOrJSONDecoder(strings.NewReader(string(crd.File.Data)), 4096)
		for {
			var document map[string]interface{}
			if err := decoder.Decode(&document); err != nil {
				if err == io.EOF {
					break
				}
				return nil, fmt.Errorf("decode CRD file %s: %w", crd.Name, err)
			}
			if len(document) == 0 {
				continue
			}
			definition, err := helmCRDDefinition(document)
			if err != nil {
				return nil, fmt.Errorf("parse CRD file %s: %w", crd.Name, err)
			}
			for _, version := range definition.ServedVersions {
				gvk := schema.GroupVersion{Group: definition.Group, Version: version}.WithKind(definition.Kind)
				definitions[gvk] = definition
			}
		}
	}
	return definitions, nil
}

func helmCRDDefinition(document map[string]interface{}) (helmChartCRDDefinition, error) {
	if kind, _ := document["kind"].(string); kind != "CustomResourceDefinition" {
		return helmChartCRDDefinition{}, fmt.Errorf("expected CustomResourceDefinition, got %q", kind)
	}
	metadata, _ := document["metadata"].(map[string]interface{})
	name, _ := metadata["name"].(string)
	spec, _ := document["spec"].(map[string]interface{})
	names, _ := spec["names"].(map[string]interface{})
	kind, _ := names["kind"].(string)
	group, _ := spec["group"].(string)
	if strings.TrimSpace(name) == "" || strings.TrimSpace(kind) == "" || strings.TrimSpace(group) == "" {
		return helmChartCRDDefinition{}, fmt.Errorf("metadata.name, spec.names.kind, and spec.group are required")
	}
	versions := make([]string, 0)
	if items, ok := spec["versions"].([]interface{}); ok {
		for _, item := range items {
			version, _ := item.(map[string]interface{})
			served, _ := version["served"].(bool)
			name, _ := version["name"].(string)
			if strings.TrimSpace(name) != "" && served {
				versions = append(versions, name)
			}
		}
	}
	if len(versions) == 0 {
		if version, _ := spec["version"].(string); strings.TrimSpace(version) != "" {
			versions = append(versions, version)
		}
	}
	if len(versions) == 0 {
		return helmChartCRDDefinition{}, fmt.Errorf("at least one served spec version is required")
	}
	return helmChartCRDDefinition{
		Name:           name,
		Group:          group,
		Kind:           kind,
		ServedVersions: versions,
	}, nil
}

func validateHelmRollbackCompatibility(ctx context.Context, actionConfig *action.Configuration, releaseName, namespace string, revision int) error {
	currentRelease, err := actionConfig.Releases.Last(releaseName)
	if err != nil {
		return fmt.Errorf("read current Helm release %s: %w", releaseName, err)
	}
	targetRevision := revision
	if targetRevision == 0 {
		targetRevision = currentRelease.Version - 1
	}
	if targetRevision <= 0 {
		return fmt.Errorf("release %s has no prior revision to roll back to", releaseName)
	}
	target, err := actionConfig.Releases.Get(releaseName, targetRevision)
	if err != nil {
		return fmt.Errorf("read Helm release %s revision %d: %w", releaseName, targetRevision, err)
	}
	if err := validateHelmReleaseCompatibility(ctx, actionConfig, releaseName, namespace, target.Chart, target.Config); err != nil {
		return err
	}
	return nil
}

func newHelmCompatibilityDryRun(actionConfig *action.Configuration, releaseName, namespace string, remoteLookup bool) *action.Install {
	dryRun := action.NewInstall(actionConfig)
	dryRun.ReleaseName = releaseName
	dryRun.Namespace = namespace
	dryRun.CreateNamespace = true
	dryRun.DryRun = true
	dryRun.DryRunOption = "client"
	if remoteLookup {
		dryRun.DryRunOption = "server"
	}
	dryRun.Timeout = helmCompatibilityTimeout
	return dryRun
}

func helmNamespaceExists(ctx context.Context, cfg *rest.Config, namespace string) (bool, error) {
	if cfg == nil {
		return false, fmt.Errorf("Helm compatibility REST config is nil")
	}
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return false, fmt.Errorf("create Helm compatibility client: %w", err)
	}
	if _, err := client.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{}); apierrors.IsNotFound(err) {
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("check Helm release namespace %s: %w", namespace, err)
	}
	return true, nil
}
