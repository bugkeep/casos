package store

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/casosorg/casos/conf"
	"gopkg.in/yaml.v3"
	"helm.sh/helm/v3/pkg/postrender"
)

var _ postrender.PostRenderer = localImagePullPolicyPostRenderer{}

const helmImplicitLatestPullPolicyConfigKey = "helmImplicitLatestPullPolicy"

func configuredLocalImagePullPolicyPostRenderer() postrender.PostRenderer {
	// This changes Kubernetes' implicit latest-tag behavior, so installations
	// opt in explicitly instead of changing existing chart defaults globally.
	if !conf.GetConfigBoolDefault(helmImplicitLatestPullPolicyConfigKey, false) {
		return nil
	}
	return localImagePullPolicyPostRenderer{}
}

// localImagePullPolicyPostRenderer only fills an omitted policy for images
// whose Kubernetes default would otherwise be Always. Explicit chart values
// remain untouched so chart authors retain control over update semantics.
type localImagePullPolicyPostRenderer struct{}

func (localImagePullPolicyPostRenderer) Run(rendered *bytes.Buffer) (*bytes.Buffer, error) {
	if rendered == nil {
		return bytes.NewBuffer(nil), nil
	}
	documents, err := helmManifestDocuments(rendered.Bytes())
	if err != nil {
		return nil, err
	}
	var output bytes.Buffer
	for _, raw := range documents {
		var document yaml.Node
		if err := yaml.Unmarshal(raw, &document); err != nil {
			return nil, fmt.Errorf("decode Helm manifest: %w", err)
		}
		if !patchPullPolicies(&document) {
			output.Write(raw)
			continue
		}
		if separator := helmManifestLeadingDocumentSeparator(raw); len(separator) > 0 {
			output.Write(separator)
		}
		encoder := yaml.NewEncoder(&output)
		encoder.SetIndent(2)
		if err := encoder.Encode(&document); err != nil {
			return nil, fmt.Errorf("encode Helm manifest: %w", err)
		}
		if err := encoder.Close(); err != nil {
			return nil, fmt.Errorf("finish Helm manifest encoding: %w", err)
		}
	}
	return &output, nil
}

func helmManifestLeadingDocumentSeparator(document []byte) []byte {
	lineEnd := bytes.IndexByte(document, '\n')
	if lineEnd < 0 {
		lineEnd = len(document)
	} else {
		lineEnd++
	}
	firstLine := document[:lineEnd]
	if !isHelmManifestDocumentSeparator(firstLine) {
		return nil
	}
	return firstLine
}

func helmManifestDocuments(rendered []byte) ([][]byte, error) {
	reader := bufio.NewReader(bytes.NewReader(rendered))
	documents := make([][]byte, 0, 1)
	var current bytes.Buffer
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			if isHelmManifestDocumentSeparator(line) && current.Len() > 0 {
				documents = append(documents, append([]byte(nil), current.Bytes()...))
				current.Reset()
			}
			current.Write(line)
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("read Helm manifest: %w", err)
		}
	}
	if current.Len() > 0 {
		documents = append(documents, current.Bytes())
	}
	return documents, nil
}

func isHelmManifestDocumentSeparator(line []byte) bool {
	line = bytes.TrimRight(line, "\r\n")
	if !bytes.HasPrefix(line, []byte("---")) {
		return false
	}
	return len(line) == 3 || line[3] == ' ' || line[3] == '\t' || line[3] == '#'
}

func patchPullPolicies(document *yaml.Node) bool {
	root := yamlDocumentRoot(document)
	if root == nil || root.Kind != yaml.MappingNode {
		return false
	}
	if scalarValue(mappingValue(root, "kind")) == "List" {
		items := mappingValue(root, "items")
		if items == nil || items.Kind != yaml.SequenceNode {
			return false
		}
		changed := false
		for _, item := range items.Content {
			changed = patchPullPolicies(item) || changed
		}
		return changed
	}

	var podSpec *yaml.Node
	switch scalarValue(mappingValue(root, "kind")) {
	case "Pod":
		podSpec = nestedMapping(root, "spec")
	case "Deployment", "StatefulSet", "DaemonSet", "ReplicaSet", "ReplicationController", "Job":
		podSpec = nestedMapping(root, "spec", "template", "spec")
	case "CronJob":
		podSpec = nestedMapping(root, "spec", "jobTemplate", "spec", "template", "spec")
	default:
		return false
	}
	if podSpec == nil {
		return false
	}
	changed := patchContainerList(podSpec, "initContainers")
	changed = patchContainerList(podSpec, "containers") || changed
	return patchContainerList(podSpec, "ephemeralContainers") || changed
}

func yamlDocumentRoot(node *yaml.Node) *yaml.Node {
	if node == nil {
		return nil
	}
	if node.Kind == yaml.DocumentNode {
		if len(node.Content) == 0 {
			return nil
		}
		return node.Content[0]
	}
	return node
}

func mappingValue(mapping *yaml.Node, key string) *yaml.Node {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil
	}
	for index := 0; index+1 < len(mapping.Content); index += 2 {
		if mapping.Content[index].Kind == yaml.ScalarNode && mapping.Content[index].Value == key {
			return mapping.Content[index+1]
		}
	}
	return nil
}

func nestedMapping(node *yaml.Node, path ...string) *yaml.Node {
	current := node
	for _, key := range path {
		current = mappingValue(current, key)
		if current == nil || current.Kind != yaml.MappingNode {
			return nil
		}
	}
	return current
}

func scalarValue(node *yaml.Node) string {
	if node == nil || node.Kind != yaml.ScalarNode {
		return ""
	}
	return node.Value
}

func patchContainerList(podSpec *yaml.Node, key string) bool {
	containers := mappingValue(podSpec, key)
	if containers == nil || containers.Kind != yaml.SequenceNode {
		return false
	}
	changed := false
	for _, container := range containers.Content {
		if container.Kind != yaml.MappingNode || mappingValue(container, "imagePullPolicy") != nil {
			continue
		}
		image := scalarValue(mappingValue(container, "image"))
		if image == "" || !usesImplicitLatest(image) {
			continue
		}
		container.Content = append(container.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "imagePullPolicy"},
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "IfNotPresent"},
		)
		changed = true
	}
	return changed
}

func usesImplicitLatest(image string) bool {
	if strings.Contains(image, "@") {
		return false
	}
	lastSlash := strings.LastIndex(image, "/")
	lastColon := strings.LastIndex(image, ":")
	return lastColon <= lastSlash || image[lastColon+1:] == "latest"
}
