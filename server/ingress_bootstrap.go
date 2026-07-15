package server

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
)

const (
	ingressControllerNamespace = "kube-system"
	ingressControllerName      = "traefik"
	ingressControllerClass     = "traefik"
	ingressControllerRevision  = "1"
)

func ingressControllerLabels() map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       ingressControllerName,
		"app.kubernetes.io/managed-by": "casos",
	}
}

func ensureIngressController(ctx context.Context, client kubernetes.Interface, cfg Config) error {
	if err := ensureNamespace(ctx, client, ingressControllerNamespace); err != nil {
		return err
	}
	if err := createOrUpdateServiceAccount(ctx, client, &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ingressControllerName,
			Namespace: ingressControllerNamespace,
			Labels:    ingressControllerLabels(),
		},
	}); err != nil {
		return err
	}
	if err := ensureIngressControllerClusterRole(ctx, client); err != nil {
		return err
	}
	if err := ensureIngressControllerClusterRoleBinding(ctx, client); err != nil {
		return err
	}
	if err := ensureIngressClass(ctx, client); err != nil {
		return err
	}
	if err := ensureIngressControllerService(ctx, client); err != nil {
		return err
	}
	return ensureIngressControllerDeployment(ctx, client, cfg)
}

func ensureIngressControllerClusterRole(ctx context.Context, client kubernetes.Interface) error {
	role := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "casos:traefik-ingress-controller",
			Labels: ingressControllerLabels(),
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"services", "endpoints", "nodes", "secrets", "namespaces"},
				Verbs:     []string{"get", "list", "watch"},
			},
			{
				APIGroups: []string{"discovery.k8s.io"},
				Resources: []string{"endpointslices"},
				Verbs:     []string{"get", "list", "watch"},
			},
			{
				APIGroups: []string{"networking.k8s.io"},
				Resources: []string{"ingresses", "ingressclasses"},
				Verbs:     []string{"get", "list", "watch"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"events"},
				Verbs:     []string{"create", "update", "patch"},
			},
			{
				APIGroups: []string{"networking.k8s.io"},
				Resources: []string{"ingresses/status"},
				Verbs:     []string{"update", "patch"},
			},
		},
	}
	return createOrUpdateClusterRole(ctx, client, role)
}

func ensureIngressControllerClusterRoleBinding(ctx context.Context, client kubernetes.Interface) error {
	binding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "casos:traefik-ingress-controller",
			Labels: ingressControllerLabels(),
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "casos:traefik-ingress-controller",
		},
		Subjects: []rbacv1.Subject{{
			Kind:      "ServiceAccount",
			Name:      ingressControllerName,
			Namespace: ingressControllerNamespace,
		}},
	}
	return createOrUpdateClusterRoleBinding(ctx, client, binding)
}

func ensureIngressClass(ctx context.Context, client kubernetes.Interface) error {
	classes := client.NetworkingV1().IngressClasses()
	desired := &networkingv1.IngressClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:   ingressControllerClass,
			Labels: ingressControllerLabels(),
			Annotations: map[string]string{
				"ingressclass.kubernetes.io/is-default-class": "true",
			},
		},
		Spec: networkingv1.IngressClassSpec{Controller: "traefik.io/ingress-controller"},
	}
	current, err := classes.Get(ctx, desired.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		if _, err := classes.Create(ctx, desired, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("create IngressClass %s: %w", desired.Name, err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("get IngressClass %s: %w", desired.Name, err)
	}
	if current.Spec.Controller != desired.Spec.Controller {
		return fmt.Errorf("IngressClass %s is owned by controller %s", desired.Name, current.Spec.Controller)
	}
	desired.ResourceVersion = current.ResourceVersion
	desired.Labels = mergeStringMap(current.Labels, desired.Labels)
	desired.Annotations = mergeStringMap(current.Annotations, desired.Annotations)
	if _, err := classes.Update(ctx, desired, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("update IngressClass %s: %w", desired.Name, err)
	}
	return nil
}

func ensureIngressControllerService(ctx context.Context, client kubernetes.Interface) error {
	desired := buildIngressControllerService()
	services := client.CoreV1().Services(ingressControllerNamespace)
	current, err := services.Get(ctx, desired.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		if _, err := services.Create(ctx, desired, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("create Ingress controller Service: %w", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("get Ingress controller Service: %w", err)
	}
	desired.ResourceVersion = current.ResourceVersion
	desired.Spec.ClusterIP = current.Spec.ClusterIP
	desired.Spec.ClusterIPs = current.Spec.ClusterIPs
	desired.Spec.IPFamilies = current.Spec.IPFamilies
	desired.Spec.IPFamilyPolicy = current.Spec.IPFamilyPolicy
	desired.Spec.ExternalIPs = current.Spec.ExternalIPs
	for i := range desired.Spec.Ports {
		for _, currentPort := range current.Spec.Ports {
			if desired.Spec.Ports[i].Name == currentPort.Name {
				desired.Spec.Ports[i].NodePort = currentPort.NodePort
				break
			}
		}
	}
	if _, err := services.Update(ctx, desired, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("update Ingress controller Service: %w", err)
	}
	return nil
}

func buildIngressControllerService() *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ingressControllerName,
			Namespace: ingressControllerNamespace,
			Labels:    ingressControllerLabels(),
		},
		Spec: corev1.ServiceSpec{
			Type:                  corev1.ServiceTypeLoadBalancer,
			Selector:              ingressControllerLabels(),
			ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyTypeCluster,
			Ports: []corev1.ServicePort{
				{Name: "web", Port: 80, TargetPort: intstr.FromInt(8000), Protocol: corev1.ProtocolTCP},
				{Name: "websecure", Port: 443, TargetPort: intstr.FromInt(8443), Protocol: corev1.ProtocolTCP},
			},
		},
	}
}

func ensureIngressControllerDeployment(ctx context.Context, client kubernetes.Interface, cfg Config) error {
	return createOrUpdateDeployment(ctx, client, buildIngressControllerDeployment(cfg))
}

func buildIngressControllerDeployment(cfg Config) *appsv1.Deployment {
	replicas := int32(1)
	image := cfg.IngressControllerImage
	if image == "" {
		image = "docker.1ms.run/traefik:v3.3.4"
	}
	labels := ingressControllerLabels()
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ingressControllerName,
			Namespace: ingressControllerNamespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
					Annotations: map[string]string{
						"casos.io/rollout-rev": ingressControllerRevision,
					},
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: ingressControllerName,
					Tolerations: []corev1.Toleration{
						{Key: "CriticalAddonsOnly", Operator: corev1.TolerationOpExists},
						{Key: "node-role.kubernetes.io/control-plane", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule},
						{Key: "node-role.kubernetes.io/master", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule},
						{Key: "casos.io/bootstrap", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule},
					},
					Containers: []corev1.Container{{
						Name:            ingressControllerName,
						Image:           image,
						ImagePullPolicy: corev1.PullIfNotPresent,
						Args: []string{
							"--providers.kubernetesingress=true",
							"--providers.kubernetesingress.ingressclass=" + ingressControllerClass,
							"--providers.kubernetesingress.ingressendpoint.publishedservice=" + ingressControllerNamespace + "/" + ingressControllerName,
							"--entrypoints.web.address=:8000",
							"--entrypoints.websecure.address=:8443",
							"--ping=true",
						},
						Ports: []corev1.ContainerPort{
							{Name: "web", ContainerPort: 8000, Protocol: corev1.ProtocolTCP},
							{Name: "websecure", ContainerPort: 8443, Protocol: corev1.ProtocolTCP},
							{Name: "ping", ContainerPort: 8080, Protocol: corev1.ProtocolTCP},
						},
						ReadinessProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{
								Path: "/ping", Port: intstr.FromInt(8080), Scheme: corev1.URISchemeHTTP,
							}},
							InitialDelaySeconds: 5, PeriodSeconds: 10, TimeoutSeconds: 5, FailureThreshold: 6,
						},
					}},
				},
			},
		},
	}
	return deployment
}
