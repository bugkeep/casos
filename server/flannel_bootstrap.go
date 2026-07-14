package server

import (
	"context"
	"fmt"
	"reflect"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	appsinternal "k8s.io/kubernetes/pkg/apis/apps/v1"
)

const (
	flannelNamespace      = "kube-flannel"
	flannelServiceAccount = "flannel"
	flannelConfigMap      = "kube-flannel-cfg"
	flannelDaemonSet      = "kube-flannel-ds"
	flannelNetwork        = "10.244.0.0/16"
)

func ensureFlannel(ctx context.Context, client kubernetes.Interface, cfg Config) error {
	if err := ensureNamespace(ctx, client, flannelNamespace); err != nil {
		return err
	}
	if err := ensureFlannelServiceAccount(ctx, client); err != nil {
		return err
	}
	if err := ensureFlannelClusterRole(ctx, client); err != nil {
		return err
	}
	if err := ensureFlannelClusterRoleBinding(ctx, client); err != nil {
		return err
	}
	if err := ensureFlannelConfigMap(ctx, client); err != nil {
		return err
	}
	return ensureFlannelDaemonSet(ctx, client, cfg)
}

func flannelLabels() map[string]string {
	return map[string]string{
		"app":                          "flannel",
		"k8s-app":                      "flannel",
		"app.kubernetes.io/name":       "flannel",
		"app.kubernetes.io/managed-by": "casos",
	}
}

func ensureFlannelServiceAccount(ctx context.Context, client kubernetes.Interface) error {
	return createOrUpdateServiceAccount(ctx, client, &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{Name: flannelServiceAccount, Namespace: flannelNamespace, Labels: flannelLabels()},
	})
}

func ensureFlannelClusterRole(ctx context.Context, client kubernetes.Interface) error {
	return createOrUpdateClusterRole(ctx, client, &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: "flannel", Labels: flannelLabels()},
		Rules: []rbacv1.PolicyRule{
			{APIGroups: []string{""}, Resources: []string{"pods", "nodes"}, Verbs: []string{"get", "list", "watch"}},
			{APIGroups: []string{""}, Resources: []string{"nodes/status"}, Verbs: []string{"patch", "update"}},
		},
	})
}

func ensureFlannelClusterRoleBinding(ctx context.Context, client kubernetes.Interface) error {
	return createOrUpdateClusterRoleBinding(ctx, client, &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "flannel", Labels: flannelLabels()},
		RoleRef:    rbacv1.RoleRef{APIGroup: "rbac.authorization.k8s.io", Kind: "ClusterRole", Name: "flannel"},
		Subjects:   []rbacv1.Subject{{Kind: "ServiceAccount", Name: flannelServiceAccount, Namespace: flannelNamespace}},
	})
}

func ensureFlannelConfigMap(ctx context.Context, client kubernetes.Interface) error {
	return createOrUpdateConfigMap(ctx, client, &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: flannelConfigMap, Namespace: flannelNamespace, Labels: flannelLabels()},
		Data: map[string]string{
			"net-conf.json": fmt.Sprintf(`{"Network":%q,"Backend":{"Type":"vxlan"}}`, flannelNetwork),
			"cni-conf.json": flannelCNIConfigData(),
		},
	})
}

func flannelCNIConfigData() string {
	return `{
  "cniVersion": "0.3.1",
  "name": "cbr0",
  "plugins": [
    {"type": "flannel", "delegate": {"bridge": "cni0", "hairpinMode": true, "isDefaultGateway": true, "ipMasq": true}},
    {"type": "portmap", "capabilities": {"portMappings": true}}
  ]
}`
}

func ensureFlannelDaemonSet(ctx context.Context, client kubernetes.Interface, cfg Config) error {
	return createOrUpdateDaemonSet(ctx, client, buildFlannelDaemonSet(cfg))
}

func buildFlannelDaemonSet(cfg Config) *appsv1.DaemonSet {
	flannelDaemonImage := cfg.FlannelImage
	if flannelDaemonImage == "" {
		flannelDaemonImage = defaultFlannelImage
	}
	flannelPluginImage := cfg.FlannelCNIPluginImage
	if flannelPluginImage == "" {
		flannelPluginImage = defaultFlannelCNIPluginImage
	}
	labels := flannelLabels()
	selector := map[string]string{"app": "flannel", "k8s-app": "flannel"}
	initCNI := corev1.Container{
		Name: "install-cni-plugin", Image: flannelPluginImage, ImagePullPolicy: corev1.PullIfNotPresent,
		Command:      []string{"cp", "/flannel", "/opt/cni/bin/flannel"},
		VolumeMounts: []corev1.VolumeMount{{Name: "cni-bin", MountPath: "/opt/cni/bin"}},
	}
	initConfig := corev1.Container{
		Name: "install-cni", Image: flannelDaemonImage, ImagePullPolicy: corev1.PullIfNotPresent,
		Command: []string{"cp"}, Args: []string{"-f", "/etc/kube-flannel/cni-conf.json", "/etc/cni/net.d/10-flannel.conflist"},
		VolumeMounts: []corev1.VolumeMount{
			{Name: "cni-conf", MountPath: "/etc/cni/net.d"},
			{Name: "flannel-cfg", MountPath: "/etc/kube-flannel", ReadOnly: true},
		},
	}
	flannel := corev1.Container{
		Name: "kube-flannel", Image: flannelDaemonImage, ImagePullPolicy: corev1.PullIfNotPresent,
		Command: []string{"/opt/bin/flanneld"}, Args: []string{"--ip-masq", "--kube-subnet-mgr"},
		Env: []corev1.EnvVar{
			{Name: "POD_NAME", ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.name"}}},
			{Name: "POD_NAMESPACE", ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.namespace"}}},
		},
		SecurityContext: &corev1.SecurityContext{Privileged: ptr(true)},
		Ports:           []corev1.ContainerPort{{Name: "vxlan", ContainerPort: 8472, Protocol: corev1.ProtocolUDP}},
		VolumeMounts:    []corev1.VolumeMount{{Name: "run", MountPath: "/run/flannel"}, {Name: "flannel-cfg", MountPath: "/etc/kube-flannel", ReadOnly: true}, {Name: "xtables-lock", MountPath: "/run/xtables.lock"}},
	}
	daemonSet := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: flannelDaemonSet, Namespace: flannelNamespace, Labels: labels},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: selector},
			Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: labels}, Spec: corev1.PodSpec{
				ServiceAccountName: flannelServiceAccount, HostNetwork: true,
				NodeSelector:   map[string]string{"kubernetes.io/os": "linux"},
				Tolerations:    []corev1.Toleration{{Operator: corev1.TolerationOpExists}},
				InitContainers: []corev1.Container{initCNI, initConfig}, Containers: []corev1.Container{flannel},
				Volumes: []corev1.Volume{
					{Name: "run", VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/run/flannel"}}},
					{Name: "cni-bin", VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/opt/cni/bin"}}},
					{Name: "cni-conf", VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/etc/cni/net.d"}}},
					{Name: "flannel-cfg", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: flannelConfigMap}}}},
					{Name: "xtables-lock", VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/run/xtables.lock", Type: ptr(corev1.HostPathFileOrCreate)}}},
				},
			}},
		},
	}
	return daemonSet
}

func createOrUpdateDaemonSet(ctx context.Context, client kubernetes.Interface, desired *appsv1.DaemonSet) error {
	appsinternal.SetObjectDefaults_DaemonSet(desired)
	current, err := client.AppsV1().DaemonSets(desired.Namespace).Get(ctx, desired.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		if _, err := client.AppsV1().DaemonSets(desired.Namespace).Create(ctx, desired, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("create daemonset %s/%s: %w", desired.Namespace, desired.Name, err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("get daemonset %s/%s: %w", desired.Namespace, desired.Name, err)
	}
	if reflect.DeepEqual(current.Labels, desired.Labels) &&
		reflect.DeepEqual(current.Annotations, desired.Annotations) &&
		reflect.DeepEqual(current.Spec, desired.Spec) {
		return nil
	}
	desired.ResourceVersion = current.ResourceVersion
	if _, err := client.AppsV1().DaemonSets(desired.Namespace).Update(ctx, desired, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("update daemonset %s/%s: %w", desired.Namespace, desired.Name, err)
	}
	return nil
}
