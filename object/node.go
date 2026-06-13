package object

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

func GetNodes(cfg *rest.Config) ([]corev1.Node, error) {
	client, err := newClient(cfg)
	if err != nil {
		return nil, err
	}
	list, err := client.CoreV1().Nodes().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func GetNode(cfg *rest.Config, name string) (*corev1.Node, error) {
	client, err := newClient(cfg)
	if err != nil {
		return nil, err
	}
	return client.CoreV1().Nodes().Get(context.Background(), name, metav1.GetOptions{})
}

func UpdateNode(cfg *rest.Config, node *corev1.Node) (*corev1.Node, error) {
	client, err := newClient(cfg)
	if err != nil {
		return nil, err
	}
	return client.CoreV1().Nodes().Update(context.Background(), node, metav1.UpdateOptions{})
}

func DeleteNode(cfg *rest.Config, name string) error {
	client, err := newClient(cfg)
	if err != nil {
		return err
	}
	return client.CoreV1().Nodes().Delete(context.Background(), name, metav1.DeleteOptions{})
}
