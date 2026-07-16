package server

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sirupsen/logrus"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/retry"
)

// Bootstrap creates cluster-wide resources required for worker-node components
// to function correctly. It is idempotent — safe to call on every startup.
func Bootstrap(ctx context.Context, cfg *rest.Config, srvCfg Config) error {
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("bootstrap client: %w", err)
	}
	if err := ensureCasbinWebhook(ctx, client, srvCfg); err != nil {
		return err
	}
	if err := ensureNodeCIDRConsistency(ctx, client); err != nil {
		return err
	}
	if err := ensureNodeProxierBinding(ctx, client); err != nil {
		return err
	}
	apiExtensionsClient, err := apiextensionsclient.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("bootstrap API extensions client: %w", err)
	}
	if err := ensureFlannel(ctx, client, srvCfg); err != nil {
		return err
	}
	if err := ensureClusterDNS(ctx, client, srvCfg); err != nil {
		return err
	}
	if srvCfg.StorageProvisionerEnabled {
		if err := ensureDefaultStorageProvisioner(ctx, client, srvCfg); err != nil {
			return err
		}
	}
	if err := ensureIngressController(ctx, client, apiExtensionsClient, srvCfg); err != nil {
		return err
	}
	// Traefik CRDs are created by the ingress bootstrap above. Refresh the
	// discovered cluster admission rules so those resources are covered during
	// the same startup, rather than only after the next restart.
	if err := ensureCasbinWebhook(ctx, client, srvCfg); err != nil {
		return err
	}
	return nil
}

func admissionFailurePolicy() admissionregv1.FailurePolicyType {
	return admissionregv1.Fail
}

func admissionNamespaceSelector() *metav1.LabelSelector {
	return &metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{{
			Key:      "kubernetes.io/metadata.name",
			Operator: metav1.LabelSelectorOpNotIn,
			Values:   []string{"kube-system", "kube-flannel", "local-path-storage"},
		}},
	}
}

// normalizeNodeCIDRs keeps the legacy PodCIDR field and the NodeIPAM source
// of truth (PodCIDRs) in sync. NodeIPAM restores allocations from PodCIDRs on
// controller-manager restart; leaving only PodCIDR populated makes it assign
// a second range to an existing node.
func normalizeNodeCIDRs(node *corev1.Node) (bool, error) {
	if node == nil {
		return false, nil
	}
	legacy := strings.TrimSpace(node.Spec.PodCIDR)
	if len(node.Spec.PodCIDRs) == 0 {
		if legacy == "" {
			return false, nil
		}
		node.Spec.PodCIDRs = []string{legacy}
		return true, nil
	}
	if legacy != "" && legacy != node.Spec.PodCIDRs[0] {
		return false, fmt.Errorf("node %s has conflicting PodCIDR fields %q and %q", node.Name, legacy, node.Spec.PodCIDRs[0])
	}
	if legacy == "" {
		node.Spec.PodCIDR = node.Spec.PodCIDRs[0]
		return true, nil
	}
	return false, nil
}

func ensureNodeCIDRConsistency(ctx context.Context, client kubernetes.Interface) error {
	nodes, err := client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("list nodes for PodCIDR consistency: %w", err)
	}
	for _, existing := range nodes.Items {
		nodeName := existing.Name
		if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
			node, err := client.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
			if apierrors.IsNotFound(err) {
				return nil
			}
			if err != nil {
				return err
			}
			changed, err := normalizeNodeCIDRs(node)
			if err != nil {
				return err
			}
			if !changed {
				return nil
			}
			_, err = client.CoreV1().Nodes().Update(ctx, node, metav1.UpdateOptions{})
			return err
		}); err != nil {
			return fmt.Errorf("normalize PodCIDR fields for node %s: %w", nodeName, err)
		}
	}
	return nil
}

func admissionRules(resourceLists []*metav1.APIResourceList) []admissionregv1.RuleWithOperations {
	namespacedScope := admissionregv1.NamespacedScope
	operations := []admissionregv1.OperationType{admissionregv1.OperationAll}
	rules := []admissionregv1.RuleWithOperations{
		{
			Operations: operations,
			Rule: admissionregv1.Rule{
				APIGroups:   []string{"*"},
				APIVersions: []string{"*"},
				Resources:   []string{"*"},
				Scope:       &namespacedScope,
			},
		},
	}

	clusterResources := map[string]map[string]map[string]struct{}{}
	for _, resourceList := range resourceLists {
		group, version := splitAdmissionGroupVersion(resourceList.GroupVersion)
		if group == "admissionregistration.k8s.io" {
			continue
		}
		groupResources := clusterResources[group]
		if groupResources == nil {
			groupResources = map[string]map[string]struct{}{}
			clusterResources[group] = groupResources
		}
		resources := groupResources[version]
		if resources == nil {
			resources = map[string]struct{}{}
			groupResources[version] = resources
		}
		for _, resource := range resourceList.APIResources {
			if resource.Namespaced || strings.Contains(resource.Name, "/") {
				continue
			}
			resources[resource.Name] = struct{}{}
		}
	}

	groups := make([]string, 0, len(clusterResources))
	for group := range clusterResources {
		groups = append(groups, group)
	}
	sort.Strings(groups)
	clusterScope := admissionregv1.ClusterScope
	for _, group := range groups {
		versions := make([]string, 0, len(clusterResources[group]))
		for version := range clusterResources[group] {
			versions = append(versions, version)
		}
		sort.Strings(versions)
		for _, version := range versions {
			resourceNames := make([]string, 0, len(clusterResources[group][version]))
			for resourceName := range clusterResources[group][version] {
				resourceNames = append(resourceNames, resourceName)
			}
			sort.Strings(resourceNames)
			if len(resourceNames) == 0 {
				continue
			}
			rules = append(rules, admissionregv1.RuleWithOperations{
				Operations: operations,
				Rule: admissionregv1.Rule{
					APIGroups:   []string{group},
					APIVersions: []string{version},
					Resources:   resourceNames,
					Scope:       &clusterScope,
				},
			})
		}
	}
	return rules
}

func splitAdmissionGroupVersion(groupVersion string) (string, string) {
	parts := strings.SplitN(groupVersion, "/", 2)
	if len(parts) == 1 {
		return "", parts[0]
	}
	return parts[0], parts[1]
}

func admissionWebhooks(url string, caData []byte, rules []admissionregv1.RuleWithOperations) []admissionregv1.ValidatingWebhook {
	sideEffects := admissionregv1.SideEffectClassNone
	workloadFailurePolicy := admissionFailurePolicy()
	clusterFailurePolicy := admissionregv1.Fail
	clientConfig := admissionregv1.WebhookClientConfig{URL: &url, CABundle: caData}
	return []admissionregv1.ValidatingWebhook{
		{
			Name:                    "admission.casbin.io",
			ClientConfig:            clientConfig,
			Rules:                   []admissionregv1.RuleWithOperations{rules[0]},
			NamespaceSelector:       admissionNamespaceSelector(),
			SideEffects:             &sideEffects,
			FailurePolicy:           &workloadFailurePolicy,
			AdmissionReviewVersions: []string{"v1"},
		},
		{
			// The discovered cluster rules exclude admissionregistration.k8s.io,
			// so this webhook can update its own configuration without weakening
			// failure handling for other cluster-scoped resources.
			Name:                    "cluster.admission.casbin.io",
			ClientConfig:            clientConfig,
			Rules:                   rules[1:],
			SideEffects:             &sideEffects,
			FailurePolicy:           &clusterFailurePolicy,
			AdmissionReviewVersions: []string{"v1"},
		},
	}
}

// ensureCasbinWebhook registers the ValidatingWebhookConfiguration that routes
// admission requests to the casos Casbin enforcement server.
func ensureCasbinWebhook(ctx context.Context, client kubernetes.Interface, cfg Config) error {
	certDir := filepath.Join(cfg.DataDir, "tls")
	caData, err := os.ReadFile(filepath.Join(certDir, "ca.crt"))
	if err != nil {
		return fmt.Errorf("read CA for webhook: %w", err)
	}

	url := fmt.Sprintf("https://127.0.0.1:%d/admission/validate", cfg.WebhookPort)
	resourceLists, discoveryErr := client.Discovery().ServerPreferredResources()
	if discoveryErr != nil {
		logrus.Warnf("partial API discovery for admission rules: %v", discoveryErr)
	}
	rules := admissionRules(resourceLists)
	if len(rules) < 2 {
		return fmt.Errorf("discover cluster-scoped API resources for admission rules")
	}
	whConfig := &admissionregv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: "casbin-admission"},
		Webhooks:   admissionWebhooks(url, caData, rules),
	}

	ar := client.AdmissionregistrationV1().ValidatingWebhookConfigurations()

	// Remove the old name left by a previous release.
	if err := ar.Delete(ctx, "casbin-gatekeeper", metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		logrus.Warnf("delete legacy casbin-gatekeeper webhook: %v", err)
	}

	existing, err := ar.Get(ctx, "casbin-admission", metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		if _, err := ar.Create(ctx, whConfig, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("create casbin-admission webhook: %w", err)
		}
		logrus.Info("created ValidatingWebhookConfiguration casbin-admission")
		return nil
	}
	if err != nil {
		return fmt.Errorf("get casbin-admission webhook: %w", err)
	}
	updated := existing.DeepCopy()
	updated.Labels = mergeStringMap(existing.Labels, whConfig.Labels)
	updated.Annotations = mergeStringMap(existing.Annotations, whConfig.Annotations)
	updated.Webhooks = whConfig.Webhooks
	if apiequality.Semantic.DeepEqual(existing.Labels, updated.Labels) &&
		apiequality.Semantic.DeepEqual(existing.Annotations, updated.Annotations) &&
		apiequality.Semantic.DeepEqual(existing.Webhooks, updated.Webhooks) {
		return nil
	}
	if _, err := ar.Update(ctx, updated, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("update casbin-admission webhook: %w", err)
	}
	logrus.Info("updated ValidatingWebhookConfiguration casbin-admission")
	return nil
}

// ensureNodeProxierBinding grants system:node-proxier to the system:nodes group
// so that kube-proxy can reuse the node kubeconfig (same file kubelet uses) to
// watch EndpointSlices and program iptables rules for NodePort services.
func ensureNodeProxierBinding(ctx context.Context, client kubernetes.Interface) error {
	const name = "casos:node-proxier"
	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "system:node-proxier",
		},
		Subjects: []rbacv1.Subject{
			{
				APIGroup: "rbac.authorization.k8s.io",
				Kind:     "Group",
				Name:     "system:nodes",
			},
		},
	}
	_, err := client.RbacV1().ClusterRoleBindings().Create(ctx, crb, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("create %s ClusterRoleBinding: %w", name, err)
	}
	logrus.Infof("created ClusterRoleBinding %s", name)
	return nil
}
