package store

import (
	"gopkg.in/yaml.v3"
)

// manifestImages collects the images out of a rendered Helm manifest. A
// document that will not parse is skipped rather than failing the whole set:
// one unreadable manifest should not blank out the images of every other
// release and quietly turn the gate off.
func manifestImages(rendered []byte) []string {
	documents, err := helmManifestDocuments(rendered)
	if err != nil {
		return nil
	}
	var images []string
	for _, raw := range documents {
		var document yaml.Node
		if err := yaml.Unmarshal(raw, &document); err != nil {
			continue
		}
		images = append(images, documentImages(&document)...)
	}
	return images
}

// documentImages walks the same workload kinds the pull-policy post-renderer
// does, so both agree on where a pod spec can be found in a chart.
func documentImages(document *yaml.Node) []string {
	root := yamlDocumentRoot(document)
	if root == nil || root.Kind != yaml.MappingNode {
		return nil
	}
	if scalarValue(mappingValue(root, "kind")) == "List" {
		items := mappingValue(root, "items")
		if items == nil || items.Kind != yaml.SequenceNode {
			return nil
		}
		var images []string
		for _, item := range items.Content {
			images = append(images, documentImages(item)...)
		}
		return images
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
		return nil
	}
	if podSpec == nil {
		return nil
	}

	var images []string
	for _, key := range []string{"initContainers", "containers", "ephemeralContainers"} {
		images = append(images, containerListImages(podSpec, key)...)
	}
	return images
}

func containerListImages(podSpec *yaml.Node, key string) []string {
	containers := mappingValue(podSpec, key)
	if containers == nil || containers.Kind != yaml.SequenceNode {
		return nil
	}
	var images []string
	for _, container := range containers.Content {
		if container.Kind != yaml.MappingNode {
			continue
		}
		if image := scalarValue(mappingValue(container, "image")); image != "" {
			images = append(images, image)
		}
	}
	return images
}
