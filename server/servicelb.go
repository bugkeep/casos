package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"reflect"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/sirupsen/logrus"
)

const (
	serviceLBManagedIPsAnnotation = "casos.io/service-lb-managed-ips"
	serviceLBDisabledAnnotation   = "casos.io/service-lb-disabled"
)

// StartServiceLB starts the built-in bare-metal LoadBalancer reconciler. It
// publishes Ready Worker addresses and uses Service externalIPs so kube-proxy
// can route LoadBalancer traffic without a cloud provider.
func StartServiceLB(ctx context.Context, cfg *rest.Config) error {
	if cfg == nil {
		return fmt.Errorf("apiserver rest config is required")
	}
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("service load balancer client: %w", err)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	go runServiceLB(ctx, client)
	return nil
}

func runServiceLB(ctx context.Context, client kubernetes.Interface) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		if err := reconcileServiceLB(ctx, client); err != nil && ctx.Err() == nil {
			logrus.Warnf("service load balancer reconciliation failed: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func reconcileServiceLB(ctx context.Context, client kubernetes.Interface) error {
	nodes, err := client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("list nodes: %w", err)
	}
	nodeIPs := readyWorkerIPs(nodes.Items)
	services, err := client.CoreV1().Services(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("list LoadBalancer services: %w", err)
	}
	for i := range services.Items {
		service := &services.Items[i]
		if service.Spec.Type != corev1.ServiceTypeLoadBalancer || service.Annotations[serviceLBDisabledAnnotation] == "true" {
			continue
		}
		if err := reconcileLoadBalancerService(ctx, client, service, nodeIPs); err != nil {
			logrus.Warnf("reconcile LoadBalancer service %s/%s failed: %v", service.Namespace, service.Name, err)
		}
	}
	return nil
}

func readyWorkerIPs(nodes []corev1.Node) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0)
	for _, node := range nodes {
		if isControlPlaneNode(node) || !isReadyWorkerNode(node) {
			continue
		}
		for _, address := range node.Status.Addresses {
			if address.Type != corev1.NodeExternalIP && address.Type != corev1.NodeInternalIP {
				continue
			}
			ip := net.ParseIP(address.Address)
			if ip == nil {
				continue
			}
			value := ip.String()
			if _, ok := seen[value]; ok {
				continue
			}
			seen[value] = struct{}{}
			result = append(result, value)
		}
	}
	return result
}

func isReadyWorkerNode(node corev1.Node) bool {
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}

func isControlPlaneNode(node corev1.Node) bool {
	_, controlPlane := node.Labels["node-role.kubernetes.io/control-plane"]
	_, master := node.Labels["node-role.kubernetes.io/master"]
	return controlPlane || master
}

func reconcileLoadBalancerService(ctx context.Context, client kubernetes.Interface, service *corev1.Service, nodeIPs []string) error {
	managedIPs, err := serviceLBManagedIPs(service.Annotations)
	if err != nil {
		return err
	}
	managedSet := make(map[string]struct{}, len(managedIPs))
	for _, ip := range managedIPs {
		managedSet[ip] = struct{}{}
	}
	userIPs := make([]string, 0, len(service.Spec.ExternalIPs))
	for _, ip := range service.Spec.ExternalIPs {
		if _, ok := managedSet[ip]; !ok && !containsString(userIPs, ip) {
			userIPs = append(userIPs, ip)
		}
	}
	desiredManagedIPs := make([]string, 0, len(nodeIPs))
	for _, ip := range nodeIPs {
		if !containsString(userIPs, ip) {
			desiredManagedIPs = append(desiredManagedIPs, ip)
		}
	}
	desiredExternalIPs := append(append([]string{}, userIPs...), nodeIPs...)
	desiredExternalIPs = uniqueStrings(desiredExternalIPs)
	encodedManagedIPs, err := json.Marshal(desiredManagedIPs)
	if err != nil {
		return fmt.Errorf("encode managed LoadBalancer IPs: %w", err)
	}
	desiredAnnotation := string(encodedManagedIPs)

	updated := service.DeepCopy()
	if !reflect.DeepEqual(updated.Spec.ExternalIPs, desiredExternalIPs) || updated.Annotations[serviceLBManagedIPsAnnotation] != desiredAnnotation {
		if updated.Annotations == nil {
			updated.Annotations = map[string]string{}
		}
		updated.Spec.ExternalIPs = desiredExternalIPs
		updated.Annotations[serviceLBManagedIPsAnnotation] = desiredAnnotation
		current, err := client.CoreV1().Services(service.Namespace).Update(ctx, updated, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("update LoadBalancer service spec: %w", err)
		}
		updated = current
	}

	desiredStatus := updated.Status.DeepCopy()
	desiredStatus.LoadBalancer.Ingress = make([]corev1.LoadBalancerIngress, 0, len(nodeIPs))
	for _, ip := range nodeIPs {
		desiredStatus.LoadBalancer.Ingress = append(desiredStatus.LoadBalancer.Ingress, corev1.LoadBalancerIngress{IP: ip})
	}
	if reflect.DeepEqual(updated.Status, *desiredStatus) {
		return nil
	}
	updated.Status = *desiredStatus
	if _, err := client.CoreV1().Services(service.Namespace).UpdateStatus(ctx, updated, metav1.UpdateOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("update LoadBalancer service status: %w", err)
	}
	return nil
}

func serviceLBManagedIPs(annotations map[string]string) ([]string, error) {
	value := annotations[serviceLBManagedIPsAnnotation]
	if value == "" {
		return nil, nil
	}
	var ips []string
	if err := json.Unmarshal([]byte(value), &ips); err != nil {
		return nil, fmt.Errorf("decode managed LoadBalancer IPs: %w", err)
	}
	return uniqueStrings(ips), nil
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func uniqueStrings(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" && !containsString(result, value) {
			result = append(result, value)
		}
	}
	return result
}
