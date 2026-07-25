package store

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/casosorg/casos/conf"
	"gopkg.in/yaml.v3"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chartutil"
)

const (
	bitnamiChartRepoURL         = "https://charts.bitnami.com/bitnami"
	bitnamiOCIChartRepoPrefix   = "oci://registry-1.docker.io/bitnamicharts/"
	bitnamiDockerOCIChartPrefix = "oci://docker.io/bitnamicharts/"
	defaultBitnamiRegistry      = "docker.1ms.run"
	defaultBitnamiOCIPrefix     = "oci://" + defaultBitnamiRegistry + "/bitnamicharts/"
	bitnamiRegistryConfigKey    = "bitnamiLegacyRegistry"
)

func bitnamiLegacyRegistry() string {
	return strings.TrimRight(conf.GetConfigStringDefault(bitnamiRegistryConfigKey, defaultBitnamiRegistry), "/")
}

func bitnamiMirrorOCIChartPrefix() string {
	return "oci://" + bitnamiLegacyRegistry() + "/bitnamicharts/"
}

func bitnamiLegacyImageWarning() string {
	return fmt.Sprintf("Bitnami legacy image fallback is enabled: images use %s/bitnamilegacy from the frozen snapshot and global.security.allowInsecureImages=true; review these values before installation", bitnamiLegacyRegistry())
}

// GetHelmChartInstallValues returns chart defaults with only the compatibility
// overrides that the install path will also enforce.
func GetHelmChartInstallValues(chartName, repoURL, version string) (string, error) {
	ch, err := loadChart(chartName, repoURL, version)
	if err != nil {
		return "", err
	}
	return renderHelmChartInstallValues(ch, repoURL)
}

func renderHelmChartInstallValues(ch *chart.Chart, repoURL string) (string, error) {
	values, fallbackApplied, err := buildHelmChartInstallValues(ch, repoURL)
	if err != nil {
		return "", err
	}
	data, err := yaml.Marshal(values)
	if err != nil {
		return "", err
	}
	if fallbackApplied {
		return "# WARNING: " + bitnamiLegacyImageWarning() + "\n" + string(data), nil
	}
	return string(data), nil
}

func buildHelmChartInstallValues(ch *chart.Chart, repoURL string) (map[string]interface{}, bool, error) {
	if ch == nil {
		return map[string]interface{}{}, false, nil
	}
	overrides, fallbackApplied, err := prepareHelmInstallValues(ch, repoURL, map[string]interface{}{})
	if err != nil {
		return nil, false, err
	}
	values := cloneHelmValues(ch.Values)
	if err := mergeHelmValueOverrides(values, overrides, nil); err != nil {
		return nil, false, err
	}
	return values, fallbackApplied, nil
}

// prepareHelmInstallValues computes compatibility changes from Helm's fully
// coalesced values, then merges only changed paths into the caller's values.
func prepareHelmInstallValues(ch *chart.Chart, repoURL string, input map[string]interface{}) (map[string]interface{}, bool, error) {
	values := cloneHelmValues(input)
	if ch == nil || !isBitnamiCommunityChartRepo(repoURL) {
		return values, false, nil
	}

	dependencyValues := cloneHelmValues(input)
	if err := chartutil.ProcessDependenciesWithMerge(ch, dependencyValues); err != nil {
		return nil, false, fmt.Errorf("process Helm chart dependencies: %w", err)
	}
	coalesced, err := chartutil.CoalesceValues(ch, dependencyValues)
	if err != nil {
		return nil, false, fmt.Errorf("coalesce Helm install values: %w", err)
	}
	before := map[string]interface{}(coalesced)
	patched := cloneHelmValues(before)
	fallbackApplied := rewriteBitnamiLegacyImageRepositories(patched, false)
	installDefaultsApplied := applyBitnamiInstallabilityDefaults(patched, input)
	if !fallbackApplied && !installDefaultsApplied {
		return values, false, nil
	}

	if fallbackApplied {
		security, err := ensureHelmValuesPath(patched, "global", "security")
		if err != nil {
			return nil, false, err
		}
		security["allowInsecureImages"] = true
	}

	if err := mergeHelmValueOverrides(values, changedHelmValues(before, patched), nil); err != nil {
		return nil, false, err
	}
	return values, fallbackApplied, nil
}

func applyBitnamiInstallabilityDefaults(values, explicitValues map[string]interface{}) bool {
	// Capability keys apply only when the chart declares them and the caller
	// did not override them, avoiding a catalog of chart-name special cases.
	changed := false
	for key, wanted := range map[string]bool{
		"tomcatInstallDefaultWebapps": true,
	} {
		if _, explicitlySet := explicitValues[key]; explicitlySet {
			continue
		}
		current, exists := values[key]
		currentBool, isBool := current.(bool)
		if exists && isBool && currentBool != wanted {
			values[key] = wanted
			changed = true
		}
	}
	return changed
}

func isBitnamiCommunityChartRepo(repoURL string) bool {
	normalized := strings.ToLower(strings.TrimRight(strings.TrimSpace(repoURL), "/"))
	return normalized == bitnamiChartRepoURL ||
		strings.HasPrefix(normalized+"/", bitnamiOCIChartRepoPrefix) ||
		strings.HasPrefix(normalized+"/", bitnamiDockerOCIChartPrefix) ||
		strings.HasPrefix(normalized+"/", defaultBitnamiOCIPrefix) ||
		strings.HasPrefix(normalized+"/", bitnamiMirrorOCIChartPrefix())
}

func ociChartPullCandidates(repoURL string) []string {
	normalized := strings.ToLower(repoURL)
	for _, sourcePrefix := range []string{bitnamiOCIChartRepoPrefix, bitnamiDockerOCIChartPrefix, defaultBitnamiOCIPrefix} {
		if strings.HasPrefix(normalized, sourcePrefix) {
			preferred := bitnamiMirrorOCIChartPrefix() + repoURL[len(sourcePrefix):]
			if strings.EqualFold(preferred, repoURL) {
				return []string{repoURL}
			}
			return []string{preferred, repoURL}
		}
	}
	return []string{repoURL}
}

func rewriteBitnamiLegacyImageRepositories(value interface{}, imageValues bool) bool {
	changed := false
	switch typed := value.(type) {
	case map[string]interface{}:
		if imageValues && rewriteBitnamiLegacyImageRepository(typed) {
			changed = true
		}
		for key, child := range typed {
			if rewriteBitnamiLegacyImageRepositories(child, isHelmImageValuesKey(key)) {
				changed = true
			}
		}
	case []interface{}:
		for _, child := range typed {
			if rewriteBitnamiLegacyImageRepositories(child, imageValues) {
				changed = true
			}
		}
	}
	return changed
}

func isHelmImageValuesKey(key string) bool {
	return strings.HasSuffix(strings.ToLower(strings.TrimSpace(key)), "image")
}

func rewriteBitnamiLegacyImageRepository(values map[string]interface{}) bool {
	repository, ok := values["repository"].(string)
	if !ok || !hasBitnamiImageVersion(values) {
		return false
	}
	registry, _ := values["registry"].(string)
	registry = strings.ToLower(strings.TrimSpace(registry))
	preferredRegistry := strings.ToLower(bitnamiLegacyRegistry())
	if registry != "" && registry != "docker.io" && registry != "registry-1.docker.io" && registry != defaultBitnamiRegistry && registry != preferredRegistry {
		return false
	}

	var imageName string
	switch {
	case strings.HasPrefix(repository, "bitnami/"):
		imageName = strings.TrimPrefix(repository, "bitnami/")
	case strings.HasPrefix(repository, "docker.io/bitnami/"):
		imageName = strings.TrimPrefix(repository, "docker.io/bitnami/")
	case strings.HasPrefix(repository, "registry-1.docker.io/bitnami/"):
		imageName = strings.TrimPrefix(repository, "registry-1.docker.io/bitnami/")
	case strings.HasPrefix(repository, "bitnamilegacy/"):
		imageName = strings.TrimPrefix(repository, "bitnamilegacy/")
	case strings.HasPrefix(repository, "docker.io/bitnamilegacy/"):
		imageName = strings.TrimPrefix(repository, "docker.io/bitnamilegacy/")
	case strings.HasPrefix(repository, "registry-1.docker.io/bitnamilegacy/"):
		imageName = strings.TrimPrefix(repository, "registry-1.docker.io/bitnamilegacy/")
	default:
		return false
	}
	wantedRepository := "bitnamilegacy/" + imageName
	if registry == preferredRegistry && repository == wantedRepository {
		return false
	}
	values["registry"] = preferredRegistry
	values["repository"] = wantedRepository
	return true
}

func hasBitnamiImageVersion(values map[string]interface{}) bool {
	if digest, ok := values["digest"].(string); ok && strings.TrimSpace(digest) != "" {
		return true
	}
	tag, ok := values["tag"].(string)
	if !ok {
		return false
	}
	return strings.TrimSpace(tag) != ""
}

func ensureHelmValuesPath(root map[string]interface{}, path ...string) (map[string]interface{}, error) {
	current := root
	for index, key := range path {
		existing, exists := current[key]
		if !exists || existing == nil {
			next := map[string]interface{}{}
			current[key] = next
			current = next
			continue
		}
		next, ok := existing.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("%s must be a map to apply Bitnami image compatibility", strings.Join(path[:index+1], "."))
		}
		current = next
	}
	return current, nil
}

func changedHelmValues(before, after map[string]interface{}) map[string]interface{} {
	changes := map[string]interface{}{}
	for key, afterValue := range after {
		beforeValue, exists := before[key]
		if !exists {
			changes[key] = cloneHelmValue(afterValue)
			continue
		}
		beforeMap, beforeIsMap := beforeValue.(map[string]interface{})
		afterMap, afterIsMap := afterValue.(map[string]interface{})
		if beforeIsMap && afterIsMap {
			if nested := changedHelmValues(beforeMap, afterMap); len(nested) != 0 {
				changes[key] = nested
			}
			continue
		}
		if !reflect.DeepEqual(beforeValue, afterValue) {
			changes[key] = cloneHelmValue(afterValue)
		}
	}
	return changes
}

func mergeHelmValueOverrides(target, overrides map[string]interface{}, path []string) error {
	for key, override := range overrides {
		currentPath := append(path, key)
		overrideMap, isMap := override.(map[string]interface{})
		if !isMap {
			target[key] = cloneHelmValue(override)
			continue
		}
		existing, exists := target[key]
		if !exists || existing == nil {
			existing = map[string]interface{}{}
			target[key] = existing
		}
		targetMap, ok := existing.(map[string]interface{})
		if !ok {
			return fmt.Errorf("%s must be a map to apply Bitnami image compatibility", strings.Join(currentPath, "."))
		}
		if err := mergeHelmValueOverrides(targetMap, overrideMap, currentPath); err != nil {
			return err
		}
	}
	return nil
}

func cloneHelmValues(values map[string]interface{}) map[string]interface{} {
	cloned := make(map[string]interface{}, len(values))
	for key, value := range values {
		cloned[key] = cloneHelmValue(value)
	}
	return cloned
}

func cloneHelmValue(value interface{}) interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		return cloneHelmValues(typed)
	case []interface{}:
		cloned := make([]interface{}, len(typed))
		for index, child := range typed {
			cloned[index] = cloneHelmValue(child)
		}
		return cloned
	default:
		return typed
	}
}
