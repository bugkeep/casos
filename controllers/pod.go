package controllers

import (
	"sync/atomic"
	"unsafe"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// adminCfg is atomically set once the apiserver is ready.
var adminCfg unsafe.Pointer // *rest.Config

// SetAdminRestConfig injects the admin rest config after apiserver bootstrap.
func SetAdminRestConfig(cfg *rest.Config) {
	atomic.StorePointer(&adminCfg, unsafe.Pointer(cfg))
}

func getAdminRestConfig() *rest.Config {
	return (*rest.Config)(atomic.LoadPointer(&adminCfg))
}

type podSummary struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Phase     string `json:"phase"`
	NodeName  string `json:"nodeName"`
}

// GetPods
// @router /api/get-pods [get]
func (c *ApiController) GetPods() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}

	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}

	pods, err := client.CoreV1().Pods("").List(c.Ctx.Request.Context(), metav1.ListOptions{})
	if err != nil {
		c.ResponseError(err.Error())
		return
	}

	result := make([]podSummary, 0, len(pods.Items))
	for _, p := range pods.Items {
		result = append(result, podSummary{
			Namespace: p.Namespace,
			Name:      p.Name,
			Phase:     string(p.Status.Phase),
			NodeName:  p.Spec.NodeName,
		})
	}
	c.ResponseOk(result)
}
