package store

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
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

func validateHelmChartCompatibility(actionConfig *action.Configuration, releaseName, namespace string, chartToInstall *chart.Chart, values map[string]interface{}) error {
	if chartToInstall == nil || chartToInstall.Metadata == nil {
		return fmt.Errorf("chart metadata is missing")
	}
	if !isInstallableHelmChartMetadata(chartToInstall.Metadata) {
		return fmt.Errorf("chart %s is a library chart and cannot be installed as an application", chartToInstall.Name())
	}
	if len(chartToInstall.CRDObjects()) > 0 {
		return fmt.Errorf("chart %s contains unsupported CustomResourceDefinition resources", chartToInstall.Name())
	}

	dryRun := action.NewInstall(actionConfig)
	dryRun.ReleaseName = releaseName
	dryRun.Namespace = namespace
	dryRun.DryRun = true
	dryRun.DryRunOption = "server"
	manifest, err := dryRun.Run(chartToInstall, values)
	if err != nil {
		return fmt.Errorf("render chart %s for compatibility check: %w", chartToInstall.Name(), err)
	}
	if err := validateHelmManifestCompatibility(manifest.Manifest); err != nil {
		return fmt.Errorf("chart %s is not compatible with the supported application profile: %w", chartToInstall.Name(), err)
	}
	return nil
}

func validateHelmManifestCompatibility(manifest string) error {
	decoder := utilyaml.NewYAMLOrJSONDecoder(strings.NewReader(manifest), 4096)
	unsupported := make(map[string]struct{})
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
		collectUnsupportedHelmKinds(document, unsupported)
	}
	if len(unsupported) == 0 {
		return nil
	}
	kinds := make([]string, 0, len(unsupported))
	for kind := range unsupported {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	return fmt.Errorf("unsupported resource kinds: %s", strings.Join(kinds, ", "))
}

func collectUnsupportedHelmKinds(document map[string]interface{}, unsupported map[string]struct{}) {
	kind, _ := document["kind"].(string)
	if kind == "List" {
		items, _ := document["items"].([]interface{})
		for _, item := range items {
			object, ok := item.(map[string]interface{})
			if ok {
				collectUnsupportedHelmKinds(object, unsupported)
			}
		}
		return
	}
	if kind == "" {
		return
	}
	if _, ok := helmCompatibilityKinds[kind]; !ok {
		unsupported[kind] = struct{}{}
	}
}
