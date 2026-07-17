package store

import (
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	bitnamiChartRepoURL       = "https://charts.bitnami.com/bitnami"
	bitnamiOCIChartRepoPrefix = "oci://registry-1.docker.io/bitnamicharts/"
)

// GetHelmChartInstallValues returns the values shown in the App Store install
// dialog. Raw chart defaults remain available through GetHelmChartDefaultValues.
func GetHelmChartInstallValues(chartName, repoURL, version string) (string, error) {
	ch, err := loadChart(chartName, repoURL, version)
	if err != nil {
		return "", err
	}

	values := cloneHelmValues(ch.Values)
	if isBitnamiCommunityChartRepo(repoURL) {
		applyBitnamiLegacyImageFallback(values)
		applyBitnamiAppStoreChartDefaults(chartName, values)
	}

	data, err := yaml.Marshal(values)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func applyBitnamiAppStoreChartDefaults(chartName string, values map[string]interface{}) {
	if strings.EqualFold(strings.TrimSpace(chartName), "tomcat") {
		values["tomcatInstallDefaultWebapps"] = true
	}
}

func isBitnamiCommunityChartRepo(repoURL string) bool {
	normalized := strings.ToLower(strings.TrimRight(strings.TrimSpace(repoURL), "/"))
	return normalized == bitnamiChartRepoURL || strings.HasPrefix(normalized+"/", bitnamiOCIChartRepoPrefix)
}

func applyBitnamiLegacyImageFallback(values map[string]interface{}) {
	if !rewriteBitnamiLegacyImageRepositories(values, false) {
		return
	}

	global := ensureHelmValuesMap(values, "global")
	security := ensureHelmValuesMap(global, "security")
	security["allowInsecureImages"] = true
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
		return true
	case strings.HasPrefix(repository, "docker.io/bitnami/"):
		values["repository"] = "docker.io/bitnamilegacy/" + strings.TrimPrefix(repository, "docker.io/bitnami/")
		return true
	case strings.HasPrefix(repository, "registry-1.docker.io/bitnami/"):
		values["repository"] = "registry-1.docker.io/bitnamilegacy/" + strings.TrimPrefix(repository, "registry-1.docker.io/bitnami/")
		return true
	default:
		return false
	}
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

func ensureHelmValuesMap(parent map[string]interface{}, key string) map[string]interface{} {
	if current, ok := parent[key].(map[string]interface{}); ok {
		return current
	}
	current := map[string]interface{}{}
	parent[key] = current
	return current
}

func cloneHelmValues(values map[string]interface{}) map[string]interface{} {
	if values == nil {
		return map[string]interface{}{}
	}
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
		for i, child := range typed {
			cloned[i] = cloneHelmValue(child)
		}
		return cloned
	default:
		return typed
	}
}
