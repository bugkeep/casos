package controllers

import (
	"encoding/json"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/casosorg/casos/object"
)

type daemonSetSummary struct {
	Namespace              string          `json:"namespace"`
	Name                   string          `json:"name"`
	Image                  string          `json:"image"`
	DesiredNumberScheduled int32           `json:"desiredNumberScheduled"`
	NumberReady            int32           `json:"numberReady"`
	EnvVars                []envVarSummary `json:"envVars"`
	CreatedAt              string          `json:"createdAt"`
	ResourceVersion        string          `json:"resourceVersion"`
}

func toDaemonSetSummary(ds appsv1.DaemonSet) daemonSetSummary {
	image := ""
	if len(ds.Spec.Template.Spec.Containers) > 0 {
		image = ds.Spec.Template.Spec.Containers[0].Image
	}
	return daemonSetSummary{
		Namespace:              ds.Namespace,
		Name:                   ds.Name,
		Image:                  image,
		DesiredNumberScheduled: ds.Status.DesiredNumberScheduled,
		NumberReady:            ds.Status.NumberReady,
		EnvVars:                extractEnvVars(ds.Spec.Template.Spec.Containers),
		CreatedAt:              ds.CreationTimestamp.UTC().Format("2006-01-02 15:04:05"),
		ResourceVersion:        ds.ResourceVersion,
	}
}

type daemonSetRequest struct {
	Namespace       string          `json:"namespace"`
	Name            string          `json:"name"`
	ContainerName   string          `json:"containerName"`
	Image           string          `json:"image"`
	CpuRequest      string          `json:"cpuRequest"`
	MemoryRequest   string          `json:"memoryRequest"`
	EnvVars         []envVarRequest `json:"envVars"`
	ResourceVersion string          `json:"resourceVersion"`
}

func buildDaemonSet(req daemonSetRequest) *appsv1.DaemonSet {
	containerName := req.ContainerName
	if containerName == "" {
		containerName = req.Name
	}

	container := corev1.Container{
		Name:  containerName,
		Image: req.Image,
		Env:   buildEnvVars(req.EnvVars),
	}

	if req.CpuRequest != "" || req.MemoryRequest != "" {
		reqs := corev1.ResourceList{}
		if req.CpuRequest != "" {
			reqs[corev1.ResourceCPU] = resource.MustParse(req.CpuRequest)
		}
		if req.MemoryRequest != "" {
			reqs[corev1.ResourceMemory] = resource.MustParse(req.MemoryRequest)
		}
		container.Resources = corev1.ResourceRequirements{Requests: reqs}
	}

	return &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.Name,
			Namespace: req.Namespace,
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": req.Name},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": req.Name},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{container},
				},
			},
		},
	}
}

// GetDaemonSets
// @router /api/get-daemonsets [get]
func (c *ApiController) GetDaemonSets() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	namespace := c.GetString("namespace")
	list, err := object.GetDaemonSets(cfg, namespace)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	result := make([]daemonSetSummary, 0, len(list))
	for _, ds := range list {
		result = append(result, toDaemonSetSummary(ds))
	}
	c.ResponseOk(result)
}

// GetDaemonSet
// @router /api/get-daemonset [get]
func (c *ApiController) GetDaemonSet() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	namespace := c.GetString("namespace")
	name := c.GetString("name")
	ds, err := object.GetDaemonSet(cfg, namespace, name)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(toDaemonSetSummary(*ds))
}

// AddDaemonSet
// @router /api/add-daemonset [post]
func (c *ApiController) AddDaemonSet() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	var req daemonSetRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}
	if req.Namespace == "" {
		req.Namespace = "default"
	}
	created, err := object.AddDaemonSet(cfg, buildDaemonSet(req))
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(toDaemonSetSummary(*created))
}

// UpdateDaemonSet
// @router /api/update-daemonset [post]
func (c *ApiController) UpdateDaemonSet() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	var req daemonSetRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}
	if req.Namespace == "" {
		req.Namespace = "default"
	}

	existing, err := object.GetDaemonSet(cfg, req.Namespace, req.Name)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}

	if len(existing.Spec.Template.Spec.Containers) > 0 {
		existing.Spec.Template.Spec.Containers[0].Image = req.Image
		existing.Spec.Template.Spec.Containers[0].Env = buildEnvVars(req.EnvVars)
	} else {
		containerName := req.ContainerName
		if containerName == "" {
			containerName = req.Name
		}
		existing.Spec.Template.Spec.Containers = []corev1.Container{{
			Name:  containerName,
			Image: req.Image,
			Env:   buildEnvVars(req.EnvVars),
		}}
	}
	existing.ResourceVersion = req.ResourceVersion

	updated, err := object.UpdateDaemonSet(cfg, existing)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(toDaemonSetSummary(*updated))
}

// DeleteDaemonSet
// @router /api/delete-daemonset [post]
func (c *ApiController) DeleteDaemonSet() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	var req daemonSetRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}
	if req.Namespace == "" {
		req.Namespace = "default"
	}
	if err := object.DeleteDaemonSet(cfg, req.Namespace, req.Name); err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk()
}
