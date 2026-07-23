package store

import (
	"bytes"
	"io"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestLocalImagePullPolicyPostRendererPreservesMultiDocumentBoundaries(t *testing.T) {
	rendered := bytes.NewBufferString(`---
# Source: chart/templates/first.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: first
spec:
  template:
    spec:
      containers:
        - name: nginx
          image: nginx:1.0
---
# Source: chart/templates/second.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: second
spec:
  template:
    spec:
      containers:
        - name: busybox
          image: busybox
`)

	output, err := (localImagePullPolicyPostRenderer{}).Run(rendered)
	if err != nil {
		t.Fatalf("render manifest: %v", err)
	}

	decoder := yaml.NewDecoder(bytes.NewReader(output.Bytes()))
	documents := make([]yaml.Node, 0, 2)
	for {
		var document yaml.Node
		if err := decoder.Decode(&document); err != nil {
			if err == io.EOF {
				break
			}
			t.Fatalf("decode rendered manifest: %v\n%s", err, output.String())
		}
		if yamlDocumentRoot(&document) != nil {
			documents = append(documents, document)
		}
	}
	if len(documents) != 2 {
		t.Fatalf("decoded %d documents, want 2\n%s", len(documents), output.String())
	}

	firstPodSpec := nestedMapping(yamlDocumentRoot(&documents[0]), "spec", "template", "spec")
	firstContainer := mappingValue(firstPodSpec, "containers").Content[0]
	if policy := mappingValue(firstContainer, "imagePullPolicy"); policy != nil {
		t.Fatalf("unchanged document gained imagePullPolicy %q", scalarValue(policy))
	}
	secondPodSpec := nestedMapping(yamlDocumentRoot(&documents[1]), "spec", "template", "spec")
	secondContainer := mappingValue(secondPodSpec, "containers").Content[0]
	if policy := scalarValue(mappingValue(secondContainer, "imagePullPolicy")); policy != "IfNotPresent" {
		t.Fatalf("second document imagePullPolicy = %q, want IfNotPresent", policy)
	}
}
