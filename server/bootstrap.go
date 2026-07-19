package server

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sirupsen/logrus"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	casbinAdmissionWebhookName = "admission.casbin.io"
	casbinAdmissionConfigName  = "casbin-admission"
)

// Bootstrap creates CasOS-managed cluster add-ons. It is idempotent and safe
// to call on every startup; individual add-ons can be disabled in config.
func Bootstrap(ctx context.Context, cfg *rest.Config, srvCfg Config) error {
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("bootstrap client: %w", err)
	}
	// The webhook config carries the CA bundle the apiserver uses to trust the
	// admission server. It must be refreshed first: if it lags behind the CA on
	// disk every admission call fails TLS verification, so it cannot be gated
	// behind the workload steps below. Those steps run independently — a failure
	// in one must not skip the others.
	errs := []error{ensureCasbinWebhook(ctx, client, srvCfg)}
	errs = append(errs, ensureNodeProxierBinding(ctx, client))
	errs = append(errs, ensureFlannel(ctx, client, srvCfg))
	errs = append(errs, ensureClusterDNS(ctx, client, srvCfg))
	if srvCfg.IngressControllerEnabled {
		errs = append(errs, ensureIngressController(ctx, client, srvCfg))
	} else {
		errs = append(errs, cleanupIngressController(ctx, client))
	}
	if !srvCfg.ServiceLBEnabled {
		errs = append(errs, cleanupServiceLB(ctx, client))
	}
	if srvCfg.StorageProvisionerEnabled {
		errs = append(errs, ensureDefaultStorageProvisioner(ctx, client, srvCfg))
	}
	return errors.Join(errs...)
}

// casbinAdmissionWebhook returns the cluster-wide webhook used by the
// co-located CasOS control plane. FailurePolicy is intentionally Ignore: the
// endpoint is served by the same process as the control plane, so failing
// closed would make a local outage prevent recovery. Callers must keep the URL
// reachable from the API server or policy enforcement is silently bypassed.
func casbinAdmissionWebhook(url string, caData []byte) admissionregv1.ValidatingWebhook {
	sideEffects := admissionregv1.SideEffectClassNoneOnDryRun
	failurePolicy := admissionregv1.Ignore
	allScopes := admissionregv1.AllScopes
	timeoutSeconds := int32(3)
	return admissionregv1.ValidatingWebhook{
		Name:         casbinAdmissionWebhookName,
		ClientConfig: admissionregv1.WebhookClientConfig{URL: &url, CABundle: caData},
		Rules: []admissionregv1.RuleWithOperations{{
			Operations: []admissionregv1.OperationType{admissionregv1.OperationAll},
			Rule: admissionregv1.Rule{
				APIGroups: []string{"*"}, APIVersions: []string{"*"}, Resources: []string{"*"}, Scope: &allScopes,
			},
		}},
		NamespaceSelector:       &metav1.LabelSelector{},
		SideEffects:             &sideEffects,
		FailurePolicy:           &failurePolicy,
		TimeoutSeconds:          &timeoutSeconds,
		AdmissionReviewVersions: []string{"v1"},
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

	url := fmt.Sprintf("https://127.0.0.1:%d%s", cfg.WebhookPort, admissionValidatePath)
	whConfig := &admissionregv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: casbinAdmissionConfigName},
		Webhooks:   []admissionregv1.ValidatingWebhook{casbinAdmissionWebhook(url, caData)},
	}

	ar := client.AdmissionregistrationV1().ValidatingWebhookConfigurations()

	// Remove the old name left by a previous release.
	if err := ar.Delete(ctx, "casbin-gatekeeper", metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		logrus.Warnf("delete legacy casbin-gatekeeper webhook: %v", err)
	}

	existing, err := ar.Get(ctx, casbinAdmissionConfigName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		if _, err := ar.Create(ctx, whConfig, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("create casbin-admission webhook: %w", err)
		}
		logrus.Infof("created ValidatingWebhookConfiguration %s", casbinAdmissionConfigName)
		return nil
	}
	if err != nil {
		return fmt.Errorf("get casbin-admission webhook: %w", err)
	}
	whConfig.ResourceVersion = existing.ResourceVersion
	if _, err := ar.Update(ctx, whConfig, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("update casbin-admission webhook: %w", err)
	}
	logrus.Infof("updated ValidatingWebhookConfiguration %s", casbinAdmissionConfigName)
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
