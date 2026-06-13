package object

import (
	"context"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

func GetClusterRoleBindings(cfg *rest.Config) ([]rbacv1.ClusterRoleBinding, error) {
	client, err := newClient(cfg)
	if err != nil {
		return nil, err
	}
	list, err := client.RbacV1().ClusterRoleBindings().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func GetClusterRoleBinding(cfg *rest.Config, name string) (*rbacv1.ClusterRoleBinding, error) {
	client, err := newClient(cfg)
	if err != nil {
		return nil, err
	}
	return client.RbacV1().ClusterRoleBindings().Get(context.Background(), name, metav1.GetOptions{})
}

func AddClusterRoleBinding(cfg *rest.Config, crb *rbacv1.ClusterRoleBinding) (*rbacv1.ClusterRoleBinding, error) {
	client, err := newClient(cfg)
	if err != nil {
		return nil, err
	}
	return client.RbacV1().ClusterRoleBindings().Create(context.Background(), crb, metav1.CreateOptions{})
}

func UpdateClusterRoleBinding(cfg *rest.Config, crb *rbacv1.ClusterRoleBinding) (*rbacv1.ClusterRoleBinding, error) {
	client, err := newClient(cfg)
	if err != nil {
		return nil, err
	}
	return client.RbacV1().ClusterRoleBindings().Update(context.Background(), crb, metav1.UpdateOptions{})
}

func DeleteClusterRoleBinding(cfg *rest.Config, name string) error {
	client, err := newClient(cfg)
	if err != nil {
		return err
	}
	return client.RbacV1().ClusterRoleBindings().Delete(context.Background(), name, metav1.DeleteOptions{})
}
