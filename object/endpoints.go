package object

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

func GetEndpoints(cfg *rest.Config, namespace, name string) (*corev1.Endpoints, error) {
	client, err := newClient(cfg)
	if err != nil {
		return nil, err
	}
	return client.CoreV1().Endpoints(namespace).Get(context.Background(), name, metav1.GetOptions{})
}
