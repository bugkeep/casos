package object

import (
	"context"

	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

func GetIngresses(cfg *rest.Config, namespace string) ([]networkingv1.Ingress, error) {
	client, err := newClient(cfg)
	if err != nil {
		return nil, err
	}
	ns := namespace
	if ns == "" {
		ns = metav1.NamespaceAll
	}
	list, err := client.NetworkingV1().Ingresses(ns).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func GetIngress(cfg *rest.Config, namespace, name string) (*networkingv1.Ingress, error) {
	client, err := newClient(cfg)
	if err != nil {
		return nil, err
	}
	return client.NetworkingV1().Ingresses(namespace).Get(context.Background(), name, metav1.GetOptions{})
}

func AddIngress(cfg *rest.Config, ing *networkingv1.Ingress) (*networkingv1.Ingress, error) {
	client, err := newClient(cfg)
	if err != nil {
		return nil, err
	}
	return client.NetworkingV1().Ingresses(ing.Namespace).Create(context.Background(), ing, metav1.CreateOptions{})
}

func UpdateIngress(cfg *rest.Config, ing *networkingv1.Ingress) (*networkingv1.Ingress, error) {
	client, err := newClient(cfg)
	if err != nil {
		return nil, err
	}
	return client.NetworkingV1().Ingresses(ing.Namespace).Update(context.Background(), ing, metav1.UpdateOptions{})
}

func DeleteIngress(cfg *rest.Config, namespace, name string) error {
	client, err := newClient(cfg)
	if err != nil {
		return err
	}
	return client.NetworkingV1().Ingresses(namespace).Delete(context.Background(), name, metav1.DeleteOptions{})
}
