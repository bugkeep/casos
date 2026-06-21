package controllers

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestBuildStatefulSetAddsPersistentVolumeClaimMount(t *testing.T) {
	req := statefulSetRequest{
		Namespace: "default",
		Name:      "repo-mysql",
		Image:     "mysql:8.0",
		PvcName:   "repo-data",
		MountPath: "/var/lib/mysql",
		ReadOnly:  true,
	}

	sts := buildStatefulSet(req)

	if len(sts.Spec.Template.Spec.Volumes) != 1 {
		t.Fatalf("expected 1 volume, got %d", len(sts.Spec.Template.Spec.Volumes))
	}
	vol := sts.Spec.Template.Spec.Volumes[0]
	if vol.PersistentVolumeClaim == nil {
		t.Fatalf("expected persistent volume claim volume")
	}
	if vol.PersistentVolumeClaim.ClaimName != "repo-data" {
		t.Fatalf("expected pvc repo-data, got %q", vol.PersistentVolumeClaim.ClaimName)
	}
	if !vol.PersistentVolumeClaim.ReadOnly {
		t.Fatalf("expected pvc volume to be readOnly")
	}

	if len(sts.Spec.Template.Spec.Containers) != 1 {
		t.Fatalf("expected 1 container, got %d", len(sts.Spec.Template.Spec.Containers))
	}
	mounts := sts.Spec.Template.Spec.Containers[0].VolumeMounts
	if len(mounts) != 1 {
		t.Fatalf("expected 1 volume mount, got %d", len(mounts))
	}
	mount := mounts[0]
	if mount.Name != vol.Name {
		t.Fatalf("expected mount name %q, got %q", vol.Name, mount.Name)
	}
	if mount.MountPath != "/var/lib/mysql" {
		t.Fatalf("expected mount path /var/lib/mysql, got %q", mount.MountPath)
	}
	if !mount.ReadOnly {
		t.Fatalf("expected mount to be readOnly")
	}
}

func TestToStatefulSetSummaryIncludesPersistentVolumeClaimMount(t *testing.T) {
	sts := appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "repo-mysql",
			Namespace: "default",
		},
		Spec: appsv1.StatefulSetSpec{
			ServiceName: "repo-mysql",
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Volumes: []corev1.Volume{{
						Name: "storage",
						VolumeSource: corev1.VolumeSource{
							PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: "repo-data",
							},
						},
					}},
					Containers: []corev1.Container{{
						Name:  "repo-mysql",
						Image: "mysql:8.0",
						VolumeMounts: []corev1.VolumeMount{{
							Name:      "storage",
							MountPath: "/var/lib/mysql",
						}},
					}},
				},
			},
		},
	}

	summary := toStatefulSetSummary(sts)

	if summary.VolumeName != "storage" {
		t.Fatalf("expected volumeName storage, got %q", summary.VolumeName)
	}
	if summary.PvcName != "repo-data" {
		t.Fatalf("expected pvcName repo-data, got %q", summary.PvcName)
	}
	if summary.MountPath != "/var/lib/mysql" {
		t.Fatalf("expected mountPath /var/lib/mysql, got %q", summary.MountPath)
	}
	if summary.ReadOnly {
		t.Fatalf("expected readOnly false")
	}
}
