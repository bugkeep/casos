package server

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sirupsen/logrus"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
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
	if err := ensureNodeCIDRConsistency(ctx, client); err != nil {
		return err
	}
	if err := ensureNodeProxierBinding(ctx, client); err != nil {
		return err
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
	if err := ensureIngressController(ctx, client, srvCfg); err != nil {
		return err
	}
	return ensureCasbinWebhook(ctx, client, srvCfg)
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

// ensureCasbinWebhook registers the ValidatingWebhookConfiguration that routes
// admission requests to the casos Casbin enforcement server.
func ensureCasbinWebhook(ctx context.Context, client kubernetes.Interface, cfg Config) error {
	certDir := filepath.Join(cfg.DataDir, "tls")
	caData, err := os.ReadFile(filepath.Join(certDir, "ca.crt"))
	if err != nil {
		return fmt.Errorf("read CA for webhook: %w", err)
	}

	url := fmt.Sprintf("https://127.0.0.1:%d/admission/validate", cfg.WebhookPort)
	sideEffects := admissionregv1.SideEffectClassNone
	failurePolicy := admissionregv1.Ignore
	all := admissionregv1.AllScopes
	whConfig := &admissionregv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: "casbin-admission"},
		Webhooks: []admissionregv1.ValidatingWebhook{
			{
				Name: "admission.casbin.io",
				ClientConfig: admissionregv1.WebhookClientConfig{
					URL:      &url,
					CABundle: caData,
				},
				Rules: []admissionregv1.RuleWithOperations{
					{
						Operations: []admissionregv1.OperationType{
							admissionregv1.OperationAll,
						},
						Rule: admissionregv1.Rule{
							APIGroups:   []string{"*"},
							APIVersions: []string{"*"},
							Resources:   []string{"*"},
							Scope:       &all,
						},
					},
				},
				NamespaceSelector:       &metav1.LabelSelector{},
				SideEffects:             &sideEffects,
				FailurePolicy:           &failurePolicy,
				AdmissionReviewVersions: []string{"v1"},
			},
		},
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
	whConfig.ResourceVersion = existing.ResourceVersion
	if _, err := ar.Update(ctx, whConfig, metav1.UpdateOptions{}); err != nil {
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
