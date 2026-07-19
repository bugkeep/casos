package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"reflect"
	"time"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	coordinationclient "k8s.io/client-go/kubernetes/typed/coordination/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/client-go/util/retry"

	"github.com/sirupsen/logrus"
)

const (
	serviceLBManagedIPsAnnotation = "casos.io/service-lb-managed-ips"
	serviceLBDisabledAnnotation   = "casos.io/service-lb-disabled"
	serviceLBEnabledAnnotation    = "casos.io/service-lb-enabled"
	serviceLBClass                = "casos.io/service-lb"
	serviceLBLeaderLease          = "casos-service-lb"
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
	coordination, err := coordinationclient.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("service load balancer coordination client: %w", err)
	}
	hostname, err := os.Hostname()
	if err != nil {
		return fmt.Errorf("service load balancer identity: %w", err)
	}
	identity := fmt.Sprintf("%s-%d", hostname, os.Getpid())
	elector, err := leaderelection.NewLeaderElector(leaderelection.LeaderElectionConfig{
		Lock: &resourcelock.LeaseLock{
			LeaseMeta:  metav1.ObjectMeta{Name: serviceLBLeaderLease, Namespace: ingressControllerNamespace},
			Client:     coordination,
			LockConfig: resourcelock.ResourceLockConfig{Identity: identity},
		},
		LeaseDuration:   15 * time.Second,
		RenewDeadline:   10 * time.Second,
		RetryPeriod:     2 * time.Second,
		ReleaseOnCancel: true,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: func(leaderCtx context.Context) { runServiceLB(leaderCtx, client) },
			OnStoppedLeading: func() { logrus.Warn("service load balancer leadership lost") },
		},
	})
	if err != nil {
		return fmt.Errorf("service load balancer leader election: %w", err)
	}
	go elector.Run(ctx)
	return nil
}

func runServiceLB(ctx context.Context, client kubernetes.Interface) {
	const interval = 5 * time.Second
	for {
		if err := reconcileServiceLB(ctx, client); err != nil && ctx.Err() == nil {
			logrus.Warnf("service load balancer reconciliation failed: %v", err)
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func reconcileServiceLB(ctx context.Context, client kubernetes.Interface) error {
	nodes, err := client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("list nodes: %w", err)
	}
	services, err := client.CoreV1().Services(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("list LoadBalancer services: %w", err)
	}
	reconcileErrors := make([]error, 0)
	for i := range services.Items {
		service := &services.Items[i]
		if service.Spec.Type != corev1.ServiceTypeLoadBalancer {
			continue
		}
		if !serviceLBManages(service) {
			if _, managed := service.Annotations[serviceLBManagedIPsAnnotation]; managed {
				if err := cleanupLoadBalancerService(ctx, client, service); err != nil {
					reconcileErrors = append(reconcileErrors, fmt.Errorf("clean up LoadBalancer service %s/%s: %w", service.Namespace, service.Name, err))
				}
			}
			continue
		}
		nodeIPs, err := serviceLBNodeIPs(ctx, client, service, nodes.Items)
		if err != nil {
			reconcileErrors = append(reconcileErrors, fmt.Errorf("select LoadBalancer nodes for %s/%s: %w", service.Namespace, service.Name, err))
			continue
		}
		if err := reconcileLoadBalancerService(ctx, client, service, nodeIPs); err != nil {
			reconcileErrors = append(reconcileErrors, fmt.Errorf("reconcile LoadBalancer service %s/%s: %w", service.Namespace, service.Name, err))
		}
	}
	return errors.Join(reconcileErrors...)
}

func cleanupServiceLB(ctx context.Context, client kubernetes.Interface) error {
	services, err := client.CoreV1().Services(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("list services for ServiceLB cleanup: %w", err)
	}
	errs := make([]error, 0)
	for i := range services.Items {
		service := &services.Items[i]
		if service.Annotations[serviceLBManagedIPsAnnotation] == "" {
			continue
		}
		if err := cleanupLoadBalancerService(ctx, client, service); err != nil {
			errs = append(errs, fmt.Errorf("clean up ServiceLB state for %s/%s: %w", service.Namespace, service.Name, err))
		}
	}
	return errors.Join(errs...)
}

func serviceLBManages(service *corev1.Service) bool {
	if service == nil || service.Spec.Type != corev1.ServiceTypeLoadBalancer || service.Annotations[serviceLBDisabledAnnotation] == "true" {
		return false
	}
	return (service.Spec.LoadBalancerClass != nil && *service.Spec.LoadBalancerClass == serviceLBClass) || service.Annotations[serviceLBEnabledAnnotation] == "true"
}

type serviceLBNode struct {
	name string
	ips  []string
}

func readyServiceLBNodes(nodes []corev1.Node) []serviceLBNode {
	return readyNodeAddresses(nodes, false)
}

func readyNodeAddresses(nodes []corev1.Node, controlPlaneOnly bool) []serviceLBNode {
	seen := map[string]struct{}{}
	result := make([]serviceLBNode, 0)
	for _, node := range nodes {
		if !isReadyNode(node) || (controlPlaneOnly != isControlPlaneNode(node)) {
			continue
		}
		externalAddresses := make([]string, 0)
		internalAddresses := make([]string, 0)
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
			if address.Type == corev1.NodeExternalIP {
				externalAddresses = append(externalAddresses, value)
			} else {
				internalAddresses = append(internalAddresses, value)
			}
		}
		addresses := externalAddresses
		if len(addresses) == 0 {
			addresses = internalAddresses
		}
		if len(addresses) > 0 {
			for _, address := range addresses {
				seen[address] = struct{}{}
			}
			result = append(result, serviceLBNode{name: node.Name, ips: addresses})
		}
	}
	return result
}

func serviceLBNodeIPs(ctx context.Context, client kubernetes.Interface, service *corev1.Service, nodes []corev1.Node) ([]string, error) {
	candidates := readyServiceLBNodes(nodes)
	if service.Spec.ExternalTrafficPolicy != corev1.ServiceExternalTrafficPolicyTypeLocal {
		ready, err := serviceHasReadyEndpoint(ctx, client, service)
		if err != nil {
			return nil, err
		}
		if !ready {
			return []string{}, nil
		}
		ips := make([]string, 0)
		for _, node := range candidates {
			ips = append(ips, node.ips...)
		}
		return uniqueStrings(ips), nil
	}
	selector := labels.SelectorFromSet(labels.Set{discoveryv1.LabelServiceName: service.Name}).String()
	endpointSlices, err := client.DiscoveryV1().EndpointSlices(service.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: selector,
	})
	if err != nil {
		return nil, fmt.Errorf("list EndpointSlices: %w", err)
	}
	localNodes := map[string]struct{}{}
	for _, endpointSlice := range endpointSlices.Items {
		for _, endpoint := range endpointSlice.Endpoints {
			if endpoint.NodeName == nil || (endpoint.Conditions.Ready != nil && !*endpoint.Conditions.Ready) {
				continue
			}
			localNodes[*endpoint.NodeName] = struct{}{}
		}
	}
	matchingIPs := func(candidates []serviceLBNode) []string {
		ips := make([]string, 0)
		for _, node := range candidates {
			if _, ok := localNodes[node.name]; !ok {
				continue
			}
			ips = append(ips, node.ips...)
		}
		return uniqueStrings(ips)
	}
	if workerIPs := matchingIPs(readyNodeAddresses(nodes, false)); len(workerIPs) > 0 {
		return workerIPs, nil
	}
	return nil, nil
}

func serviceHasReadyEndpoint(ctx context.Context, client kubernetes.Interface, service *corev1.Service) (bool, error) {
	selector := labels.SelectorFromSet(labels.Set{discoveryv1.LabelServiceName: service.Name}).String()
	endpointSlices, err := client.DiscoveryV1().EndpointSlices(service.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: selector,
	})
	if err != nil {
		return false, fmt.Errorf("list EndpointSlices: %w", err)
	}
	for _, endpointSlice := range endpointSlices.Items {
		for _, endpoint := range endpointSlice.Endpoints {
			if endpoint.Conditions.Ready == nil || *endpoint.Conditions.Ready {
				return true, nil
			}
		}
	}
	return false, nil
}

func isReadyNode(node corev1.Node) bool {
	if node.Spec.Unschedulable {
		return false
	}
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}

func cleanupLoadBalancerService(ctx context.Context, client kubernetes.Interface, service *corev1.Service) error {
	if service == nil || service.DeletionTimestamp != nil {
		return nil
	}
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		current, err := client.CoreV1().Services(service.Namespace).Get(ctx, service.Name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if current.DeletionTimestamp != nil {
			return nil
		}
		managedIPs, err := serviceLBManagedIPs(current.Annotations)
		if err != nil {
			return err
		}
		managedSet := make(map[string]struct{}, len(managedIPs))
		for _, ip := range managedIPs {
			managedSet[ip] = struct{}{}
		}
		status := current.Status.DeepCopy()
		status.LoadBalancer.Ingress = status.LoadBalancer.Ingress[:0]
		for _, ingress := range current.Status.LoadBalancer.Ingress {
			if _, managed := managedSet[ingress.IP]; !managed {
				status.LoadBalancer.Ingress = append(status.LoadBalancer.Ingress, ingress)
			}
		}
		if !reflect.DeepEqual(current.Status, *status) {
			statusUpdate := current.DeepCopy()
			statusUpdate.Status = *status
			current, err = client.CoreV1().Services(current.Namespace).UpdateStatus(ctx, statusUpdate, metav1.UpdateOptions{})
			if err != nil {
				return err
			}
		}
		updated := current.DeepCopy()
		updated.Spec.ExternalIPs = updated.Spec.ExternalIPs[:0]
		for _, ip := range current.Spec.ExternalIPs {
			if _, managed := managedSet[ip]; !managed {
				updated.Spec.ExternalIPs = append(updated.Spec.ExternalIPs, ip)
			}
		}
		delete(updated.Annotations, serviceLBManagedIPsAnnotation)
		if reflect.DeepEqual(current.Spec.ExternalIPs, updated.Spec.ExternalIPs) && current.Annotations[serviceLBManagedIPsAnnotation] == "" {
			return nil
		}
		_, err = client.CoreV1().Services(current.Namespace).Update(ctx, updated, metav1.UpdateOptions{})
		return err
	})
}

func isControlPlaneNode(node corev1.Node) bool {
	_, controlPlane := node.Labels["node-role.kubernetes.io/control-plane"]
	_, master := node.Labels["node-role.kubernetes.io/master"]
	return controlPlane || master
}

func reconcileLoadBalancerService(ctx context.Context, client kubernetes.Interface, service *corev1.Service, nodeIPs []string) error {
	if service == nil || service.DeletionTimestamp != nil {
		return nil
	}
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		current, err := client.CoreV1().Services(service.Namespace).Get(ctx, service.Name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if current.DeletionTimestamp != nil || !serviceLBManages(current) {
			return nil
		}
		return reconcileLoadBalancerServiceOnce(ctx, client, current, nodeIPs)
	})
}

func reconcileLoadBalancerServiceOnce(ctx context.Context, client kubernetes.Interface, service *corev1.Service, nodeIPs []string) error {
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
	unmanagedIngress := make([]corev1.LoadBalancerIngress, 0, len(desiredStatus.LoadBalancer.Ingress))
	for _, ingress := range desiredStatus.LoadBalancer.Ingress {
		if _, managed := managedSet[ingress.IP]; !managed {
			unmanagedIngress = append(unmanagedIngress, ingress)
		}
	}
	desiredStatus.LoadBalancer.Ingress = unmanagedIngress
	for _, ip := range nodeIPs {
		if !containsLoadBalancerIngress(desiredStatus.LoadBalancer.Ingress, ip) {
			desiredStatus.LoadBalancer.Ingress = append(desiredStatus.LoadBalancer.Ingress, corev1.LoadBalancerIngress{IP: ip})
		}
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

func containsLoadBalancerIngress(ingresses []corev1.LoadBalancerIngress, ip string) bool {
	for _, ingress := range ingresses {
		if ingress.IP == ip {
			return true
		}
	}
	return false
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
