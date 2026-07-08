package server

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
)

const (
	clusterDNSNamespace = "kube-system"
	clusterDNSName      = "coredns"
	clusterDNSServiceIP = "10.43.0.10"
	coreDNSRolloutRev   = "3"
)

func ensureClusterDNS(ctx context.Context, client kubernetes.Interface) error {
	if err := ensureNamespace(ctx, client, clusterDNSNamespace); err != nil {
		return err
	}
	if err := ensureCoreDNSServiceAccount(ctx, client); err != nil {
		return err
	}
	if err := ensureCoreDNSClusterRole(ctx, client); err != nil {
		return err
	}
	if err := ensureCoreDNSClusterRoleBinding(ctx, client); err != nil {
		return err
	}
	if err := ensureCoreDNSConfigMap(ctx, client); err != nil {
		return err
	}
	if err := ensureCoreDNSService(ctx, client); err != nil {
		return err
	}
	return ensureCoreDNSDeployment(ctx, client)
}

func ensureCoreDNSServiceAccount(ctx context.Context, client kubernetes.Interface) error {
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clusterDNSName,
			Namespace: clusterDNSNamespace,
			Labels:    coreDNSLabels(),
		},
	}
	return createOrUpdateServiceAccount(ctx, client, sa)
}

func ensureCoreDNSClusterRole(ctx context.Context, client kubernetes.Interface) error {
	role := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "system:coredns",
			Labels: coreDNSLabels(),
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"endpoints", "services", "pods", "namespaces"},
				Verbs:     []string{"list", "watch"},
			},
			{
				APIGroups: []string{"discovery.k8s.io"},
				Resources: []string{"endpointslices"},
				Verbs:     []string{"list", "watch"},
			},
		},
	}
	return createOrUpdateClusterRole(ctx, client, role)
}

func ensureCoreDNSClusterRoleBinding(ctx context.Context, client kubernetes.Interface) error {
	binding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "system:coredns",
			Labels: coreDNSLabels(),
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "system:coredns",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      clusterDNSName,
				Namespace: clusterDNSNamespace,
			},
		},
	}
	return createOrUpdateClusterRoleBinding(ctx, client, binding)
}

func ensureCoreDNSConfigMap(ctx context.Context, client kubernetes.Interface) error {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clusterDNSName,
			Namespace: clusterDNSNamespace,
			Labels:    coreDNSLabels(),
		},
		Data: map[string]string{
			"Corefile": `.:53 {
    errors
    health
    ready
    kubernetes cluster.local in-addr.arpa ip6.arpa {
        pods insecure
        fallthrough in-addr.arpa ip6.arpa
        ttl 30
    }
    forward . /etc/resolv.conf
    cache 30
    loop
    reload
    loadbalance
}
`,
		},
	}
	return createOrUpdateConfigMap(ctx, client, cm)
}

func ensureCoreDNSService(ctx context.Context, client kubernetes.Interface) error {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kube-dns",
			Namespace: clusterDNSNamespace,
			Labels:    coreDNSLabels(),
		},
		Spec: corev1.ServiceSpec{
			ClusterIP: clusterDNSServiceIP,
			Selector:  coreDNSLabels(),
			Ports: []corev1.ServicePort{
				{Name: "dns", Port: 53, Protocol: corev1.ProtocolUDP, TargetPort: intstr.FromInt(53)},
				{Name: "dns-tcp", Port: 53, Protocol: corev1.ProtocolTCP, TargetPort: intstr.FromInt(53)},
				{Name: "metrics", Port: 9153, Protocol: corev1.ProtocolTCP, TargetPort: intstr.FromInt(9153)},
			},
		},
	}
	return createOrUpdateService(ctx, client, svc)
}

func ensureCoreDNSDeployment(ctx context.Context, client kubernetes.Interface) error {
	replicas := int32(1)
	maxUnavailable := intstr.FromInt(1)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clusterDNSName,
			Namespace: clusterDNSNamespace,
			Labels:    coreDNSLabels(),
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: coreDNSLabels()},
			Strategy: appsv1.DeploymentStrategy{
				Type: appsv1.RollingUpdateDeploymentStrategyType,
				RollingUpdate: &appsv1.RollingUpdateDeployment{
					MaxUnavailable: &maxUnavailable,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: coreDNSLabels(),
					Annotations: map[string]string{
						"prometheus.io/port":   "9153",
						"prometheus.io/scrape": "true",
						"casos.io/rollout-rev": coreDNSRolloutRev,
					},
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: clusterDNSName,
					PriorityClassName:  "system-cluster-critical",
					DNSPolicy:          corev1.DNSDefault,
					Tolerations: []corev1.Toleration{
						{Key: "CriticalAddonsOnly", Operator: corev1.TolerationOpExists},
						{Key: "node-role.kubernetes.io/control-plane", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule},
						{Key: "node-role.kubernetes.io/master", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule},
					},
					Containers: []corev1.Container{
						{
							Name:            clusterDNSName,
							Image:           "docker.1ms.run/coredns/coredns:1.12.4",
							ImagePullPolicy: corev1.PullIfNotPresent,
							Args:            []string{"-conf", "/etc/coredns/Corefile"},
							Ports: []corev1.ContainerPort{
								{Name: "dns", ContainerPort: 53, Protocol: corev1.ProtocolUDP},
								{Name: "dns-tcp", ContainerPort: 53, Protocol: corev1.ProtocolTCP},
								{Name: "metrics", ContainerPort: 9153, Protocol: corev1.ProtocolTCP},
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path:   "/health",
										Port:   intstr.FromInt(8080),
										Scheme: corev1.URISchemeHTTP,
									},
								},
								InitialDelaySeconds: 10,
								TimeoutSeconds:      5,
								SuccessThreshold:    1,
								FailureThreshold:    5,
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path:   "/ready",
										Port:   intstr.FromInt(8181),
										Scheme: corev1.URISchemeHTTP,
									},
								},
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("100m"),
									corev1.ResourceMemory: resource.MustParse("70Mi"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("170Mi"),
								},
							},
							SecurityContext: &corev1.SecurityContext{
								AllowPrivilegeEscalation: ptr(false),
								ReadOnlyRootFilesystem:   ptr(true),
								RunAsNonRoot:             ptr(true),
								RunAsUser:                ptr(int64(65534)),
								RunAsGroup:               ptr(int64(65534)),
								Capabilities: &corev1.Capabilities{
									Add:  []corev1.Capability{"NET_BIND_SERVICE"},
									Drop: []corev1.Capability{"ALL"},
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{Name: "config-volume", MountPath: "/etc/coredns", ReadOnly: true},
								{Name: "tmp", MountPath: "/tmp"},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "config-volume",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{Name: clusterDNSName},
									Items: []corev1.KeyToPath{
										{Key: "Corefile", Path: "Corefile"},
									},
								},
							},
						},
						{
							Name: "tmp",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
					},
				},
			},
		},
	}
	return createOrUpdateDeployment(ctx, client, deployment)
}

func coreDNSLabels() map[string]string {
	return map[string]string{
		"k8s-app":                      "kube-dns",
		"app.kubernetes.io/name":       "coredns",
		"app.kubernetes.io/managed-by": "casos",
	}
}

func createOrUpdateService(ctx context.Context, client kubernetes.Interface, svc *corev1.Service) error {
	current, err := client.CoreV1().Services(svc.Namespace).Get(ctx, svc.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = client.CoreV1().Services(svc.Namespace).Create(ctx, svc, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("create service %s/%s: %w", svc.Namespace, svc.Name, err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("get service %s/%s: %w", svc.Namespace, svc.Name, err)
	}
	svc.ResourceVersion = current.ResourceVersion
	if current.Spec.ClusterIP != "" {
		svc.Spec.ClusterIP = current.Spec.ClusterIP
	}
	if len(current.Spec.ClusterIPs) > 0 {
		svc.Spec.ClusterIPs = current.Spec.ClusterIPs
	}
	if _, err := client.CoreV1().Services(svc.Namespace).Update(ctx, svc, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("update service %s/%s: %w", svc.Namespace, svc.Name, err)
	}
	return nil
}
