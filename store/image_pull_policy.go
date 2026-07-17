package store

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"helm.sh/helm/v3/pkg/postrender"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"

	sigsyaml "sigs.k8s.io/yaml"
)

var _ postrender.PostRenderer = localImagePullPolicyPostRenderer{}

// localImagePullPolicyPostRenderer only fills an omitted policy for images
// whose Kubernetes default would otherwise be Always. Explicit chart values
// remain untouched so chart authors retain control over update semantics.
type localImagePullPolicyPostRenderer struct{}

func (localImagePullPolicyPostRenderer) Run(rendered *bytes.Buffer) (*bytes.Buffer, error) {
	decoder := utilyaml.NewYAMLOrJSONDecoder(rendered, 4096)
	documents := make([][]byte, 0)
	for {
		var document map[string]interface{}
		if err := decoder.Decode(&document); err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("decode Helm manifest: %w", err)
		}
		if len(document) == 0 {
			continue
		}
		patchPullPolicies(document)
		encoded, err := sigsyaml.Marshal(document)
		if err != nil {
			return nil, fmt.Errorf("encode Helm manifest: %w", err)
		}
		documents = append(documents, encoded)
	}
	return bytes.NewBuffer(bytes.Join(documents, []byte("---\n"))), nil
}

func patchPullPolicies(document map[string]interface{}) {
	if kind, _ := document["kind"].(string); kind == "List" {
		items, _ := document["items"].([]interface{})
		for _, item := range items {
			if object, ok := item.(map[string]interface{}); ok {
				patchPullPolicies(object)
			}
		}
		return
	}

	kind, _ := document["kind"].(string)
	var podSpec map[string]interface{}
	switch kind {
	case "Pod":
		podSpec = nestedMap(document, "spec")
	case "Deployment", "StatefulSet", "DaemonSet", "ReplicaSet", "Job":
		podSpec = nestedMap(document, "spec", "template", "spec")
	case "CronJob":
		podSpec = nestedMap(document, "spec", "jobTemplate", "spec", "template", "spec")
	default:
		return
	}
	if podSpec == nil {
		return
	}
	patchContainerList(podSpec, "initContainers")
	patchContainerList(podSpec, "containers")
}

func nestedMap(value map[string]interface{}, path ...string) map[string]interface{} {
	current := value
	for _, key := range path {
		next, ok := current[key].(map[string]interface{})
		if !ok {
			return nil
		}
		current = next
	}
	return current
}

func patchContainerList(podSpec map[string]interface{}, key string) {
	containers, _ := podSpec[key].([]interface{})
	for _, item := range containers {
		container, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		policy, hasPolicy := container["imagePullPolicy"].(string)
		if hasPolicy && policy != "" {
			continue
		}
		image, _ := container["image"].(string)
		if image != "" && usesImplicitLatest(image) {
			container["imagePullPolicy"] = "IfNotPresent"
		}
	}
}

func usesImplicitLatest(image string) bool {
	if strings.Contains(image, "@") {
		return false
	}
	lastSlash := strings.LastIndex(image, "/")
	lastColon := strings.LastIndex(image, ":")
	return lastColon <= lastSlash || image[lastColon+1:] == "latest"
}
