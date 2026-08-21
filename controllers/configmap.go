package controllers

import (
	"encoding/json"
	"sort"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/casosorg/casos/object"
)

type configMapSummary struct {
	Namespace       string   `json:"namespace"`
	Name            string   `json:"name"`
	DataKeys        int      `json:"dataKeys"`
	Keys            []string `json:"keys"`
	CreatedAt       string   `json:"createdAt"`
	ResourceVersion string   `json:"resourceVersion"`
}

// The list view only ever renders key names, so the summary carries names and
// the values stay behind the single-object endpoints. Every namespace holds a
// kube-root-ca.crt, and app ConfigMaps hold whole config files, so a
// cluster-wide list of values runs to megabytes the browser never displays.
type configMapDetail struct {
	configMapSummary
	Data map[string]string `json:"data"`
}

func toSummary(cm corev1.ConfigMap) configMapSummary {
	keys := make([]string, 0, len(cm.Data))
	for key := range cm.Data {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return configMapSummary{
		Namespace:       cm.Namespace,
		Name:            cm.Name,
		DataKeys:        len(cm.Data),
		Keys:            keys,
		CreatedAt:       cm.CreationTimestamp.UTC().Format("2006-01-02 15:04:05"),
		ResourceVersion: cm.ResourceVersion,
	}
}

func toDetail(cm corev1.ConfigMap) configMapDetail {
	return configMapDetail{configMapSummary: toSummary(cm), Data: cm.Data}
}

// GetConfigMaps
// @router /api/get-configmaps [get]
func (c *ApiController) GetConfigMaps() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	namespace := c.GetString("namespace")
	page, limit, paged := parsePagination(c)
	cms, err := object.GetConfigMaps(cfg, namespace)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	result := make([]configMapSummary, 0, len(cms))
	for _, cm := range cms {
		result = append(result, toSummary(cm))
	}
	// Stable order keeps a given object from drifting between pages when the
	// caller paginates while traffic is mutating the namespace.
	sort.Slice(result, func(i, j int) bool {
		if result[i].Namespace != result[j].Namespace {
			return result[i].Namespace < result[j].Namespace
		}
		return result[i].Name < result[j].Name
	})
	if paged {
		items, total := paginateSlice(result, page, limit)
		c.ResponseOk(map[string]any{
			"items": items,
			"total": total,
			"page":  page,
			"limit": limit,
		})
		return
	}
	c.ResponseOk(result)
}

// GetConfigMap
// @router /api/get-configmap [get]
func (c *ApiController) GetConfigMap() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	namespace := c.GetString("namespace")
	name := c.GetString("name")
	cm, err := object.GetConfigMap(cfg, namespace, name)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(toDetail(*cm))
}

type configMapRequest struct {
	Namespace       string            `json:"namespace"`
	Name            string            `json:"name"`
	Data            map[string]string `json:"data"`
	ResourceVersion string            `json:"resourceVersion"`
}

// AddConfigMap
// @router /api/add-configmap [post]
func (c *ApiController) AddConfigMap() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	var req configMapRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}
	if req.Namespace == "" {
		req.Namespace = "default"
	}
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.Name,
			Namespace: req.Namespace,
		},
		Data: req.Data,
	}
	created, err := object.AddConfigMap(cfg, cm)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(toDetail(*created))
}

// UpdateConfigMap
// @router /api/update-configmap [post]
func (c *ApiController) UpdateConfigMap() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	var req configMapRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}
	if req.Namespace == "" {
		req.Namespace = "default"
	}
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:            req.Name,
			Namespace:       req.Namespace,
			ResourceVersion: req.ResourceVersion,
		},
		Data: req.Data,
	}
	updated, err := object.UpdateConfigMap(cfg, cm)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(toDetail(*updated))
}

// DeleteConfigMap
// @router /api/delete-configmap [post]
func (c *ApiController) DeleteConfigMap() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	var req configMapRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}
	if req.Namespace == "" {
		req.Namespace = "default"
	}
	if err := object.DeleteConfigMap(cfg, req.Namespace, req.Name); err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk()
}
