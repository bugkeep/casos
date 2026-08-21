package deploy

import (
	"strings"
	"testing"
)

func TestKubeletServiceEnablesSharedMountPropagation(t *testing.T) {
	service := kubeletService("worker-1")
	if !strings.Contains(service, "ExecStartPre=/bin/mount --make-rshared /") {
		t.Fatalf("kubelet service must prepare CSI mount propagation:\n%s", service)
	}
	if !strings.Contains(service, "--hostname-override=worker-1") {
		t.Fatalf("kubelet service lost the node name:\n%s", service)
	}
}
