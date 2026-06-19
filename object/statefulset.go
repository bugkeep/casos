package object

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

func GetStatefulSets(cfg *rest.Config, namespace string) ([]appsv1.StatefulSet, error) {
	client, err := newClient(cfg)
	if err != nil {
		return nil, err
	}
	ns := namespace
	if ns == "" {
		ns = metav1.NamespaceAll
	}
	list, err := client.AppsV1().StatefulSets(ns).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func GetStatefulSet(cfg *rest.Config, namespace, name string) (*appsv1.StatefulSet, error) {
	client, err := newClient(cfg)
	if err != nil {
		return nil, err
	}
	return client.AppsV1().StatefulSets(namespace).Get(context.Background(), name, metav1.GetOptions{})
}

func AddStatefulSet(cfg *rest.Config, sts *appsv1.StatefulSet) (*appsv1.StatefulSet, error) {
	client, err := newClient(cfg)
	if err != nil {
		return nil, err
	}
	return client.AppsV1().StatefulSets(sts.Namespace).Create(context.Background(), sts, metav1.CreateOptions{})
}

func UpdateStatefulSet(cfg *rest.Config, sts *appsv1.StatefulSet) (*appsv1.StatefulSet, error) {
	client, err := newClient(cfg)
	if err != nil {
		return nil, err
	}
	return client.AppsV1().StatefulSets(sts.Namespace).Update(context.Background(), sts, metav1.UpdateOptions{})
}

func DeleteStatefulSet(cfg *rest.Config, namespace, name string) error {
	client, err := newClient(cfg)
	if err != nil {
		return err
	}
	return client.AppsV1().StatefulSets(namespace).Delete(context.Background(), name, metav1.DeleteOptions{})
}
