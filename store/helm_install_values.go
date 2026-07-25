package store

import (
	"fmt"
	"reflect"
	"strings"

	"gopkg.in/yaml.v3"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chartutil"
)

const (
	bitnamiChartRepoURL       = "https://charts.bitnami.com/bitnami"
	bitnamiOCIChartRepoPrefix = "oci://registry-1.docker.io/bitnamicharts/"
	bitnamiLegacyImageWarning = "Bitnami legacy image fallback is enabled: images use the frozen bitnamilegacy snapshot and global.security.allowInsecureImages=true; review these values before installation"
)

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
		return "# WARNING: " + bitnamiLegacyImageWarning + "\n" + string(data), nil
	}
	return string(data), nil
}

func buildHelmChartInstallValues(ch *chart.Chart, repoURL string) (map[string]interface{}, bool, error) {
	if ch == nil {
		return map[string]interface{}{}, false, nil
	}
	return prepareHelmInstallValues(ch, repoURL, ch.Values)
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
	if !rewriteBitnamiLegacyImageRepositories(patched, false) {
		return values, false, nil
	}

	security, err := ensureHelmValuesPath(patched, "global", "security")
	if err != nil {
		return nil, false, err
	}
	security["allowInsecureImages"] = true

	if err := mergeHelmValueOverrides(values, changedHelmValues(before, patched), nil); err != nil {
		return nil, false, err
	}
	return values, true, nil
}

func isBitnamiCommunityChartRepo(repoURL string) bool {
	normalized := strings.ToLower(strings.TrimRight(strings.TrimSpace(repoURL), "/"))
	return normalized == bitnamiChartRepoURL || strings.HasPrefix(normalized+"/", bitnamiOCIChartRepoPrefix)
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
	if !ok || !usesVersionedBitnamiImage(values) {
		return false
	}
	registry, _ := values["registry"].(string)
	registry = strings.ToLower(strings.TrimSpace(registry))
	if registry != "" && registry != "docker.io" && registry != "registry-1.docker.io" {
		return false
	}

	switch {
	case strings.HasPrefix(repository, "bitnami/"):
		values["repository"] = "bitnamilegacy/" + strings.TrimPrefix(repository, "bitnami/")
	case strings.HasPrefix(repository, "docker.io/bitnami/"):
		values["repository"] = "docker.io/bitnamilegacy/" + strings.TrimPrefix(repository, "docker.io/bitnami/")
	case strings.HasPrefix(repository, "registry-1.docker.io/bitnami/"):
		values["repository"] = "registry-1.docker.io/bitnamilegacy/" + strings.TrimPrefix(repository, "registry-1.docker.io/bitnami/")
	default:
		return false
	}
	return true
}

func usesVersionedBitnamiImage(values map[string]interface{}) bool {
	if digest, ok := values["digest"].(string); ok && strings.TrimSpace(digest) != "" {
		return true
	}
	tag, ok := values["tag"].(string)
	if !ok {
		return false
	}
	tag = strings.TrimSpace(tag)
	return tag != "" && !strings.EqualFold(tag, "latest")
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
