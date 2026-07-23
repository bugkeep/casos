package store

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestUsesImplicitLatest(t *testing.T) {
	tests := []struct {
		image string
		want  bool
	}{
		{image: "nginx", want: true},
		{image: "nginx:latest", want: true},
		{image: "registry.example:5000/nginx", want: true},
		{image: "registry.example:5000/nginx:1.2", want: false},
		{image: "nginx@sha256:deadbeef", want: false},
		{image: "registry.example:5000/nginx@sha256:deadbeef", want: false},
	}
	for _, test := range tests {
		t.Run(test.image, func(t *testing.T) {
			if got := usesImplicitLatest(test.image); got != test.want {
				t.Fatalf("usesImplicitLatest(%q) = %v, want %v", test.image, got, test.want)
			}
		})
	}
}

func TestLocalImagePullPolicyPostRenderer(t *testing.T) {
	manifest := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: images
spec:
  template:
    spec:
      initContainers:
        - name: implicit
          image: registry.example:5000/init
      containers:
        - name: implicit
          image: nginx
        - name: latest
          image: nginx:latest
        - name: tagged
          image: nginx:1.2
        - name: digest
          image: nginx@sha256:deadbeef
        - name: explicit
          image: busybox
          imagePullPolicy: Always
        - name: explicit-empty
          image: alpine
          imagePullPolicy: ""
`

	output := renderPullPolicies(t, manifest)
	policies := containerPolicies(t, output)
	for _, image := range []string{"registry.example:5000/init", "nginx", "nginx:latest"} {
		if policies[image] != "IfNotPresent" {
			t.Fatalf("expected %q to use IfNotPresent, got %q", image, policies[image])
		}
	}
	for _, image := range []string{"nginx:1.2", "nginx@sha256:deadbeef", "alpine"} {
		if policies[image] != "" {
			t.Fatalf("did not expect %q policy to change, got %q", image, policies[image])
		}
	}
	if policies["busybox"] != "Always" {
		t.Fatalf("expected explicit policy to remain Always, got %q", policies["busybox"])
	}
}

func TestLocalImagePullPolicyPostRendererHandlesDocumentShapes(t *testing.T) {
	manifest := `apiVersion: v1
kind: List
items:
  - apiVersion: v1
    kind: Pod
    spec:
      containers:
        - name: pod
          image: pod-image
---
apiVersion: batch/v1
kind: CronJob
spec:
  jobTemplate:
    spec:
      template:
        spec:
          restartPolicy: Never
          containers:
            - name: cron
              image: cron-image:latest
---
apiVersion: v1
kind: ReplicationController
spec:
  template:
    spec:
      containers:
        - name: rc
          image: rc-image
---
apiVersion: v1
kind: Pod
spec:
  ephemeralContainers:
    - name: debug
      image: debug-image
`

	output := renderPullPolicies(t, manifest)
	if got := strings.Count(output, "imagePullPolicy: IfNotPresent"); got != 4 {
		t.Fatalf("expected four patched policies, got %d\n%s", got, output)
	}
	if got := countYAMLDocuments(t, output); got != 4 {
		t.Fatalf("expected four YAML documents, got %d", got)
	}
}

func TestLocalImagePullPolicyPostRendererPreservesUnrelatedScalars(t *testing.T) {
	manifest := `# retain this comment
apiVersion: example.io/v1
kind: Example
spec:
  largeInteger: 9007199254740993
  quotedInteger: "9007199254740993"
  template:
    spec:
      containers:
        - name: custom
          image: custom-image
`

	output := renderPullPolicies(t, manifest)
	for _, value := range []string{"# retain this comment", "largeInteger: 9007199254740993", `quotedInteger: "9007199254740993"`} {
		if !strings.Contains(output, value) {
			t.Fatalf("expected output to preserve %q\n%s", value, output)
		}
	}
	if strings.Contains(output, "imagePullPolicy") {
		t.Fatalf("did not expect an unknown custom resource to be patched\n%s", output)
	}
}

func TestLocalImagePullPolicyPostRendererRejectsMalformedYAML(t *testing.T) {
	renderer := localImagePullPolicyPostRenderer{}
	if _, err := renderer.Run(bytes.NewBufferString("kind: [Deployment")); err == nil {
		t.Fatal("expected malformed YAML to be rejected")
	}
}

func renderPullPolicies(t *testing.T, manifest string) string {
	t.Helper()
	renderer := localImagePullPolicyPostRenderer{}
	rendered, err := renderer.Run(bytes.NewBufferString(manifest))
	if err != nil {
		t.Fatalf("render manifest: %v", err)
	}
	return rendered.String()
}

func containerPolicies(t *testing.T, manifest string) map[string]string {
	t.Helper()
	var document yaml.Node
	if err := yaml.Unmarshal([]byte(manifest), &document); err != nil {
		t.Fatalf("decode rendered manifest: %v", err)
	}
	podSpec := nestedMapping(yamlDocumentRoot(&document), "spec", "template", "spec")
	policies := map[string]string{}
	for _, key := range []string{"initContainers", "containers", "ephemeralContainers"} {
		containers := mappingValue(podSpec, key)
		if containers == nil {
			continue
		}
		for _, container := range containers.Content {
			policies[scalarValue(mappingValue(container, "image"))] = scalarValue(mappingValue(container, "imagePullPolicy"))
		}
	}
	return policies
}

func countYAMLDocuments(t *testing.T, manifest string) int {
	t.Helper()
	decoder := yaml.NewDecoder(strings.NewReader(manifest))
	count := 0
	for {
		var document yaml.Node
		err := decoder.Decode(&document)
		if err == io.EOF {
			return count
		}
		if err != nil {
			t.Fatalf("decode rendered documents: %v", err)
		}
		if yamlDocumentRoot(&document) != nil {
			count++
		}
	}
}
