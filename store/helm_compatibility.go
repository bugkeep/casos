package store

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chartutil"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
)

var helmCompatibilityKinds = map[string]struct{}{
	"ClusterRole":                    {},
	"ClusterRoleBinding":             {},
	"ConfigMap":                      {},
	"CronJob":                        {},
	"DaemonSet":                      {},
	"Deployment":                     {},
	"EndpointSlice":                  {},
	"Endpoints":                      {},
	"HorizontalPodAutoscaler":        {},
	"Ingress":                        {},
	"Job":                            {},
	"Lease":                          {},
	"LimitRange":                     {},
	"Namespace":                      {},
	"NetworkPolicy":                  {},
	"PersistentVolumeClaim":          {},
	"Pod":                            {},
	"PodDisruptionBudget":            {},
	"PriorityClass":                  {},
	"ResourceQuota":                  {},
	"Role":                           {},
	"RoleBinding":                    {},
	"Secret":                         {},
	"Service":                        {},
	"ServiceAccount":                 {},
	"StatefulSet":                    {},
	"StorageClass":                   {},
	"ValidatingWebhookConfiguration": {},
	"MutatingWebhookConfiguration":   {},
}

var helmWorkloadKinds = map[string]struct{}{
	"CronJob":     {},
	"DaemonSet":   {},
	"Deployment":  {},
	"Job":         {},
	"Pod":         {},
	"StatefulSet": {},
}

func validateHelmChartCompatibility(actionConfig *action.Configuration, releaseName, namespace string, chartToInstall *chart.Chart, values map[string]interface{}) error {
	if chartToInstall == nil || chartToInstall.Metadata == nil {
		return fmt.Errorf("chart metadata is missing")
	}
	if chartToInstall.Metadata.Deprecated {
		return fmt.Errorf("chart %s is deprecated and cannot be installed as a supported application", chartToInstall.Name())
	}
	if !isInstallableHelmChartMetadata(chartToInstall.Metadata) {
		return fmt.Errorf("chart %s is a library chart and cannot be installed as an application", chartToInstall.Name())
	}
	crdCapabilities, err := readHelmChartCRDCapabilities(chartToInstall)
	if err != nil {
		return fmt.Errorf("read chart %s CRD capabilities: %w", chartToInstall.Name(), err)
	}
	dryRunConfig := actionConfig
	if len(crdCapabilities.kinds) > 0 {
		if actionConfig == nil {
			return fmt.Errorf("render chart %s for compatibility check: Helm action configuration is missing", chartToInstall.Name())
		}
		configCopy := *actionConfig
		dryRunConfig = &configCopy
	}
	dryRun := action.NewInstall(dryRunConfig)
	dryRun.ReleaseName = releaseName
	dryRun.Namespace = namespace
	dryRun.DryRun = true
	if len(crdCapabilities.kinds) > 0 {
		dryRun.ClientOnly = true
		dryRun.DryRunOption = "client"
		if actionConfig.Capabilities != nil {
			kubeVersion := actionConfig.Capabilities.KubeVersion
			dryRun.KubeVersion = &kubeVersion
			dryRun.APIVersions = append(chartutil.VersionSet{}, actionConfig.Capabilities.APIVersions...)
		}
		dryRun.APIVersions = append(dryRun.APIVersions, crdCapabilities.apiVersions...)
	} else {
		dryRun.DryRunOption = "server"
	}
	manifest, err := dryRun.Run(chartToInstall, values)
	if err != nil {
		return fmt.Errorf("render chart %s for compatibility check: %w", chartToInstall.Name(), err)
	}
	if err := validateHelmManifestCompatibilityWithKinds(manifest.Manifest, crdCapabilities.kinds); err != nil {
		return fmt.Errorf("chart %s is not compatible with the supported application profile: %w", chartToInstall.Name(), err)
	}
	return nil
}

func validateHelmManifestCompatibility(manifest string) error {
	return validateHelmManifestCompatibilityWithKinds(manifest, nil)
}

func validateHelmManifestCompatibilityWithKinds(manifest string, additionalKinds map[string]struct{}) error {
	decoder := utilyaml.NewYAMLOrJSONDecoder(strings.NewReader(manifest), 4096)
	unsupported := make(map[string]struct{})
	hasWorkload := false
	for {
		var document map[string]interface{}
		if err := decoder.Decode(&document); err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("decode rendered Helm manifest: %w", err)
		}
		if len(document) == 0 {
			continue
		}
		collectUnsupportedHelmKinds(document, unsupported, additionalKinds)
		if containsHelmWorkload(document) {
			hasWorkload = true
		}
	}
	if len(unsupported) > 0 {
		kinds := make([]string, 0, len(unsupported))
		for kind := range unsupported {
			kinds = append(kinds, kind)
		}
		sort.Strings(kinds)
		return fmt.Errorf("unsupported resource kinds: %s", strings.Join(kinds, ", "))
	}
	if !hasWorkload {
		return fmt.Errorf("no workload resources were rendered; configure the chart's required values before installing")
	}
	return nil
}

type helmCRDCapabilities struct {
	kinds       map[string]struct{}
	apiVersions chartutil.VersionSet
}

func helmChartCRDKinds(ch *chart.Chart) (map[string]struct{}, error) {
	capabilities, err := readHelmChartCRDCapabilities(ch)
	if err != nil {
		return nil, err
	}
	return capabilities.kinds, nil
}

func readHelmChartCRDCapabilities(ch *chart.Chart) (helmCRDCapabilities, error) {
	capabilities := helmCRDCapabilities{
		kinds: make(map[string]struct{}),
	}
	if ch == nil {
		return capabilities, nil
	}
	apiVersions := make(map[string]struct{})
	for _, crd := range ch.CRDObjects() {
		if crd.File == nil {
			return capabilities, fmt.Errorf("CRD file %s is missing", crd.Name)
		}
		decoder := utilyaml.NewYAMLOrJSONDecoder(strings.NewReader(string(crd.File.Data)), 4096)
		for {
			var document map[string]interface{}
			if err := decoder.Decode(&document); err != nil {
				if err == io.EOF {
					break
				}
				return capabilities, fmt.Errorf("decode CRD file %s: %w", crd.Name, err)
			}
			if len(document) == 0 {
				continue
			}
			kind, group, versions, err := helmCRDDefinition(document)
			if err != nil {
				return capabilities, fmt.Errorf("parse CRD file %s: %w", crd.Name, err)
			}
			capabilities.kinds[kind] = struct{}{}
			for _, version := range versions {
				apiVersions[group+"/"+version] = struct{}{}
			}
		}
	}
	for apiVersion := range apiVersions {
		capabilities.apiVersions = append(capabilities.apiVersions, apiVersion)
	}
	sort.Strings(capabilities.apiVersions)
	return capabilities, nil
}

func helmCRDDefinition(document map[string]interface{}) (string, string, []string, error) {
	if kind, _ := document["kind"].(string); kind != "CustomResourceDefinition" {
		return "", "", nil, fmt.Errorf("expected CustomResourceDefinition, got %q", kind)
	}
	spec, _ := document["spec"].(map[string]interface{})
	names, _ := spec["names"].(map[string]interface{})
	kind, _ := names["kind"].(string)
	group, _ := spec["group"].(string)
	if strings.TrimSpace(kind) == "" || strings.TrimSpace(group) == "" {
		return "", "", nil, fmt.Errorf("spec.names.kind and spec.group are required")
	}
	versions := make([]string, 0)
	for _, item := range interfaceSlice(spec["versions"]) {
		version, _ := item.(map[string]interface{})
		served, hasServed := version["served"].(bool)
		name, _ := version["name"].(string)
		if strings.TrimSpace(name) != "" && (!hasServed || served) {
			versions = append(versions, name)
		}
	}
	if len(versions) == 0 {
		if version, _ := spec["version"].(string); strings.TrimSpace(version) != "" {
			versions = append(versions, version)
		}
	}
	if len(versions) == 0 {
		return "", "", nil, fmt.Errorf("at least one served spec version is required")
	}
	return kind, group, versions, nil
}

func interfaceSlice(value interface{}) []interface{} {
	items, _ := value.([]interface{})
	return items
}

func containsHelmWorkload(document map[string]interface{}) bool {
	kind, _ := document["kind"].(string)
	if kind == "List" {
		items, _ := document["items"].([]interface{})
		for _, item := range items {
			object, ok := item.(map[string]interface{})
			if ok && containsHelmWorkload(object) {
				return true
			}
		}
		return false
	}
	_, ok := helmWorkloadKinds[kind]
	return ok
}

func collectUnsupportedHelmKinds(document map[string]interface{}, unsupported, additionalKinds map[string]struct{}) {
	kind, _ := document["kind"].(string)
	if kind == "List" {
		items, _ := document["items"].([]interface{})
		for _, item := range items {
			object, ok := item.(map[string]interface{})
			if ok {
				collectUnsupportedHelmKinds(object, unsupported, additionalKinds)
			}
		}
		return
	}
	if kind == "" {
		return
	}
	if _, ok := helmCompatibilityKinds[kind]; ok {
		return
	}
	if _, ok := additionalKinds[kind]; ok {
		return
	}
	unsupported[kind] = struct{}{}
}
