package object

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

func GetDaemonSets(cfg *rest.Config, namespace string) ([]appsv1.DaemonSet, error) {
	client, err := newClient(cfg)
	if err != nil {
		return nil, err
	}
	ns := namespace
	if ns == "" {
		ns = metav1.NamespaceAll
	}
	list, err := client.AppsV1().DaemonSets(ns).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func GetDaemonSet(cfg *rest.Config, namespace, name string) (*appsv1.DaemonSet, error) {
	client, err := newClient(cfg)
	if err != nil {
		return nil, err
	}
	return client.AppsV1().DaemonSets(namespace).Get(context.Background(), name, metav1.GetOptions{})
}

func AddDaemonSet(cfg *rest.Config, ds *appsv1.DaemonSet) (*appsv1.DaemonSet, error) {
	client, err := newClient(cfg)
	if err != nil {
		return nil, err
	}
	return client.AppsV1().DaemonSets(ds.Namespace).Create(context.Background(), ds, metav1.CreateOptions{})
}

func UpdateDaemonSet(cfg *rest.Config, ds *appsv1.DaemonSet) (*appsv1.DaemonSet, error) {
	client, err := newClient(cfg)
	if err != nil {
		return nil, err
	}
	return client.AppsV1().DaemonSets(ds.Namespace).Update(context.Background(), ds, metav1.UpdateOptions{})
}

func DeleteDaemonSet(cfg *rest.Config, namespace, name string) error {
	client, err := newClient(cfg)
	if err != nil {
		return err
	}
	return client.AppsV1().DaemonSets(namespace).Delete(context.Background(), name, metav1.DeleteOptions{})
}
