package controllers

import (
	"encoding/json"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/casosorg/casos/object"
)

type statefulSetSummary struct {
	Namespace       string          `json:"namespace"`
	Name            string          `json:"name"`
	ServiceName     string          `json:"serviceName"`
	Replicas        int32           `json:"replicas"`
	ReadyReplicas   int32           `json:"readyReplicas"`
	Image           string          `json:"image"`
	EnvVars         []envVarSummary `json:"envVars"`
	VolumeName      string          `json:"volumeName"`
	PvcName         string          `json:"pvcName"`
	MountPath       string          `json:"mountPath"`
	ReadOnly        bool            `json:"readOnly"`
	CreatedAt       string          `json:"createdAt"`
	ResourceVersion string          `json:"resourceVersion"`
}

func toStatefulSetSummary(sts appsv1.StatefulSet) statefulSetSummary {
	image := ""
	volumeName := ""
	pvcName := ""
	mountPath := ""
	readOnly := false
	if len(sts.Spec.Template.Spec.Containers) > 0 {
		image = sts.Spec.Template.Spec.Containers[0].Image
		if len(sts.Spec.Template.Spec.Containers[0].VolumeMounts) > 0 {
			mount := sts.Spec.Template.Spec.Containers[0].VolumeMounts[0]
			volumeName = mount.Name
			mountPath = mount.MountPath
			readOnly = mount.ReadOnly
		}
	}
	for _, vol := range sts.Spec.Template.Spec.Volumes {
		if vol.Name == volumeName && vol.PersistentVolumeClaim != nil {
			pvcName = vol.PersistentVolumeClaim.ClaimName
			if !readOnly {
				readOnly = vol.PersistentVolumeClaim.ReadOnly
			}
			break
		}
	}
	replicas := int32(1)
	if sts.Spec.Replicas != nil {
		replicas = *sts.Spec.Replicas
	}
	return statefulSetSummary{
		Namespace:       sts.Namespace,
		Name:            sts.Name,
		ServiceName:     sts.Spec.ServiceName,
		Replicas:        replicas,
		ReadyReplicas:   sts.Status.ReadyReplicas,
		Image:           image,
		EnvVars:         extractEnvVars(sts.Spec.Template.Spec.Containers),
		VolumeName:      volumeName,
		PvcName:         pvcName,
		MountPath:       mountPath,
		ReadOnly:        readOnly,
		CreatedAt:       sts.CreationTimestamp.UTC().Format("2006-01-02 15:04:05"),
		ResourceVersion: sts.ResourceVersion,
	}
}

type statefulSetRequest struct {
	Namespace       string          `json:"namespace"`
	Name            string          `json:"name"`
	ServiceName     string          `json:"serviceName"`
	ContainerName   string          `json:"containerName"`
	Replicas        int32           `json:"replicas"`
	Image           string          `json:"image"`
	CpuRequest      string          `json:"cpuRequest"`
	MemoryRequest   string          `json:"memoryRequest"`
	EnvVars         []envVarRequest `json:"envVars"`
	VolumeName      string          `json:"volumeName"`
	PvcName         string          `json:"pvcName"`
	MountPath       string          `json:"mountPath"`
	ReadOnly        bool            `json:"readOnly"`
	ResourceVersion string          `json:"resourceVersion"`
}

func buildStatefulSet(req statefulSetRequest) *appsv1.StatefulSet {
	replicas := req.Replicas
	if replicas <= 0 {
		replicas = 1
	}
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
	volumes := []corev1.Volume{}

	if req.PvcName != "" && req.MountPath != "" {
		volumeName := req.VolumeName
		if volumeName == "" {
			volumeName = "storage"
		}
		volumes = append(volumes, corev1.Volume{
			Name: volumeName,
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: req.PvcName,
					ReadOnly:  req.ReadOnly,
				},
			},
		})
		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
			Name:      volumeName,
			MountPath: req.MountPath,
			ReadOnly:  req.ReadOnly,
		})
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
					Volumes:    volumes,
					Containers: []corev1.Container{container},
				},
			},
		},
	}
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
	created, err := object.AddStatefulSet(cfg, buildStatefulSet(req))
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

	replicas := req.Replicas
	if replicas <= 0 {
		replicas = 1
	}
	existing.Spec.Replicas = &replicas
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
	if req.PvcName != "" && req.MountPath != "" {
		volumeName := req.VolumeName
		if volumeName == "" {
			volumeName = "storage"
		}
		existing.Spec.Template.Spec.Volumes = []corev1.Volume{{
			Name: volumeName,
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: req.PvcName,
					ReadOnly:  req.ReadOnly,
				},
			},
		}}
		existing.Spec.Template.Spec.Containers[0].VolumeMounts = []corev1.VolumeMount{{
			Name:      volumeName,
			MountPath: req.MountPath,
			ReadOnly:  req.ReadOnly,
		}}
	} else {
		existing.Spec.Template.Spec.Volumes = nil
		existing.Spec.Template.Spec.Containers[0].VolumeMounts = nil
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
