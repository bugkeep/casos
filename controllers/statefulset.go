package controllers

import (
	"encoding/json"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/casosorg/casos/object"
)

type statefulSetSummary struct {
	Namespace     string `json:"namespace"`
	Name          string `json:"name"`
	ServiceName   string `json:"serviceName"`
	Replicas      int32  `json:"replicas"`
	ReadyReplicas int32  `json:"readyReplicas"`
	Image         string `json:"image"`
	resourceSummary
	Selector        map[string]string `json:"selector"`
	EnvVars         []envVarSummary   `json:"envVars"`
	CreatedAt       string            `json:"createdAt"`
	ResourceVersion string            `json:"resourceVersion"`
}

func toStatefulSetSummary(sts appsv1.StatefulSet) statefulSetSummary {
	image := ""
	if len(sts.Spec.Template.Spec.Containers) > 0 {
		image = sts.Spec.Template.Spec.Containers[0].Image
	}
	replicas := int32(1)
	if sts.Spec.Replicas != nil {
		replicas = *sts.Spec.Replicas
	}
	selector := map[string]string{}
	if sts.Spec.Selector != nil {
		selector = sts.Spec.Selector.MatchLabels
	}
	return statefulSetSummary{
		Namespace:       sts.Namespace,
		Name:            sts.Name,
		ServiceName:     sts.Spec.ServiceName,
		Replicas:        replicas,
		ReadyReplicas:   sts.Status.ReadyReplicas,
		Image:           image,
		resourceSummary: extractResources(sts.Spec.Template.Spec.Containers),
		Selector:        selector,
		EnvVars:         extractEnvVars(sts.Spec.Template.Spec.Containers),
		CreatedAt:       sts.CreationTimestamp.UTC().Format("2006-01-02 15:04:05"),
		ResourceVersion: sts.ResourceVersion,
	}
}

type statefulSetRequest struct {
	Namespace     string `json:"namespace"`
	Name          string `json:"name"`
	ServiceName   string `json:"serviceName"`
	ContainerName string `json:"containerName"`
	Replicas      *int32 `json:"replicas"`
	Image         string `json:"image"`
	resourceRequest
	EnvVars         []envVarRequest `json:"envVars"`
	ResourceVersion string          `json:"resourceVersion"`
}

func buildStatefulSet(req statefulSetRequest) (*appsv1.StatefulSet, error) {
	replicas := replicasOrDefault(req.Replicas)
	containerName := req.ContainerName
	if containerName == "" {
		containerName = req.Name
	}
	serviceName := req.ServiceName
	if serviceName == "" {
		serviceName = req.Name
	}

	container := corev1.Container{
		Name:  containerName,
		Image: req.Image,
		Env:   buildEnvVars(req.EnvVars),
	}

	if err := applyResources(&container, req.resourceRequest); err != nil {
		return nil, err
	}

	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:            req.Name,
			Namespace:       req.Namespace,
			ResourceVersion: req.ResourceVersion,
		},
		Spec: appsv1.StatefulSetSpec{
			ServiceName: serviceName,
			Replicas:    &replicas,
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
	}, nil
}

// GetStatefulSets
// @router /api/get-statefulsets [get]
func (c *ApiController) GetStatefulSets() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	namespace := c.GetString("namespace")
	list, err := object.GetStatefulSets(cfg, namespace)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	result := make([]statefulSetSummary, 0, len(list))
	for _, sts := range list {
		result = append(result, toStatefulSetSummary(sts))
	}
	c.ResponseOk(result)
}

// GetStatefulSet
// @router /api/get-statefulset [get]
func (c *ApiController) GetStatefulSet() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	namespace := c.GetString("namespace")
	name := c.GetString("name")
	sts, err := object.GetStatefulSet(cfg, namespace, name)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(toStatefulSetSummary(*sts))
}

// AddStatefulSet
// @router /api/add-statefulset [post]
func (c *ApiController) AddStatefulSet() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	var req statefulSetRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}
	if req.Namespace == "" {
		req.Namespace = "default"
	}
	statefulSet, err := buildStatefulSet(req)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	created, err := object.AddStatefulSet(cfg, statefulSet)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(toStatefulSetSummary(*created))
}

// UpdateStatefulSet
// @router /api/update-statefulset [post]
func (c *ApiController) UpdateStatefulSet() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	var req statefulSetRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}
	if req.Namespace == "" {
		req.Namespace = "default"
	}

	existing, err := object.GetStatefulSet(cfg, req.Namespace, req.Name)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}

	// A payload that leaves out the replica count keeps the one the workload is
	// already running at; only an explicit value scales it, zero included.
	if req.Replicas != nil {
		replicas := replicasOrDefault(req.Replicas)
		existing.Spec.Replicas = &replicas
	}
	if len(existing.Spec.Template.Spec.Containers) == 0 {
		c.ResponseError(fmt.Sprintf("stateful set %s/%s has no containers", req.Namespace, req.Name))
		return
	}
	container := &existing.Spec.Template.Spec.Containers[0]
	container.Image = req.Image
	container.Env = buildEnvVars(req.EnvVars)
	if err := applyResources(container, req.resourceRequest); err != nil {
		c.ResponseError(err.Error())
		return
	}
	existing.ResourceVersion = req.ResourceVersion

	updated, err := object.UpdateStatefulSet(cfg, existing)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(toStatefulSetSummary(*updated))
}

// DeleteStatefulSet
// @router /api/delete-statefulset [post]
func (c *ApiController) DeleteStatefulSet() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	var req statefulSetRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}
	if req.Namespace == "" {
		req.Namespace = "default"
	}
	if err := object.DeleteStatefulSet(cfg, req.Namespace, req.Name); err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk()
}
