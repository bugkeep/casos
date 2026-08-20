package controllers

import "testing"

func int32Ptr(v int32) *int32 {
	return &v
}

func TestReplicasOrDefault(t *testing.T) {
	cases := []struct {
		name      string
		requested *int32
		want      int32
	}{
		{name: "absent field takes the Kubernetes default", requested: nil, want: 1},
		{name: "explicit zero scales to zero", requested: int32Ptr(0), want: 0},
		{name: "explicit count is kept", requested: int32Ptr(3), want: 3},
		{name: "negative count floors at zero", requested: int32Ptr(-2), want: 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := replicasOrDefault(c.requested); got != c.want {
				t.Errorf("replicasOrDefault() = %d, want %d", got, c.want)
			}
		})
	}
}

func TestBuildWorkloadsKeepZeroReplicas(t *testing.T) {
	depl := buildDeployment(deploymentRequest{Namespace: "default", Name: "coredns", Image: "coredns:1.11", Replicas: int32Ptr(0)})
	if depl.Spec.Replicas == nil || *depl.Spec.Replicas != 0 {
		t.Errorf("deployment replicas = %v, want 0", depl.Spec.Replicas)
	}

	sts := buildStatefulSet(statefulSetRequest{Namespace: "default", Name: "etcd", Image: "etcd:3.5", Replicas: int32Ptr(0)})
	if sts.Spec.Replicas == nil || *sts.Spec.Replicas != 0 {
		t.Errorf("statefulset replicas = %v, want 0", sts.Spec.Replicas)
	}
}
