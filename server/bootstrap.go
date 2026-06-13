package server

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// Bootstrap creates cluster-wide resources required for worker-node components
// to function correctly. It is idempotent — safe to call on every startup.
func Bootstrap(ctx context.Context, cfg *rest.Config) error {
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("bootstrap client: %w", err)
	}
	return ensureNodeProxierBinding(ctx, client)
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
