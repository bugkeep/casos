package store

import (
	"bytes"
	"testing"

	sigsyaml "sigs.k8s.io/yaml"
)

func TestLocalImagePullPolicyPostRenderer(t *testing.T) {
	renderer := localImagePullPolicyPostRenderer{}
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
`

	rendered, err := renderer.Run(bytes.NewBufferString(manifest))
	if err != nil {
		t.Fatalf("render manifest: %v", err)
	}
	var output struct {
		Spec struct {
			Template struct {
				Spec struct {
					InitContainers []struct {
						Image           string `yaml:"image"`
						ImagePullPolicy string `yaml:"imagePullPolicy"`
					} `yaml:"initContainers"`
					Containers []struct {
						Image           string `yaml:"image"`
						ImagePullPolicy string `yaml:"imagePullPolicy"`
					} `yaml:"containers"`
				} `yaml:"spec"`
			} `yaml:"template"`
		} `yaml:"spec"`
	}
	if err := sigsyaml.Unmarshal(rendered.Bytes(), &output); err != nil {
		t.Fatalf("decode rendered manifest: %v", err)
	}
	policies := map[string]string{}
	for _, container := range output.Spec.Template.Spec.InitContainers {
		policies[container.Image] = container.ImagePullPolicy
	}
	for _, container := range output.Spec.Template.Spec.Containers {
		policies[container.Image] = container.ImagePullPolicy
	}

	for _, image := range []string{"registry.example:5000/init", "nginx", "nginx:latest"} {
		if policies[image] != "IfNotPresent" {
			t.Fatalf("expected %q to use IfNotPresent, got %q", image, policies[image])
		}
	}
	for _, image := range []string{"nginx:1.2", "nginx@sha256:deadbeef"} {
		if policies[image] != "" {
			t.Fatalf("did not expect %q policy to change, got %q", image, policies[image])
		}
	}
	if policies["busybox"] != "Always" {
		t.Fatalf("expected explicit policy to remain Always, got %q", policies["busybox"])
	}
}
