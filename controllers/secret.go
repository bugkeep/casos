package controllers

import (
	"encoding/json"
	"sort"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/casosorg/casos/object"
)

type secretSummary struct {
	Namespace       string   `json:"namespace"`
	Name            string   `json:"name"`
	Type            string   `json:"type"`
	DataKeys        int      `json:"dataKeys"`
	Keys            []string `json:"keys"`
	CreatedAt       string   `json:"createdAt"`
	ResourceVersion string   `json:"resourceVersion"`
}

// Listing every Secret used to hand the browser every service-account token,
// registry credential and TLS private key in the cluster, none of which the
// list view renders. The values stay behind the single-object endpoints.
type secretDetail struct {
	secretSummary
	StringData map[string]string `json:"stringData"`
}

func toSecretSummary(s corev1.Secret) secretSummary {
	keys := make([]string, 0, len(s.Data))
	for key := range s.Data {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return secretSummary{
		Namespace:       s.Namespace,
		Name:            s.Name,
		Type:            string(s.Type),
		DataKeys:        len(s.Data),
		Keys:            keys,
		CreatedAt:       s.CreationTimestamp.UTC().Format("2006-01-02 15:04:05"),
		ResourceVersion: s.ResourceVersion,
	}
}

func toSecretDetail(s corev1.Secret) secretDetail {
	sd := make(map[string]string, len(s.Data))
	for k, v := range s.Data {
		sd[k] = string(v)
	}
	return secretDetail{secretSummary: toSecretSummary(s), StringData: sd}
}

// GetSecrets
// @router /api/get-secrets [get]
func (c *ApiController) GetSecrets() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	namespace := c.GetString("namespace")
	secrets, err := object.GetSecrets(cfg, namespace)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	result := make([]secretSummary, 0, len(secrets))
	for _, s := range secrets {
		result = append(result, toSecretSummary(s))
	}
	c.ResponseOk(result)
}

// GetSecret
// @router /api/get-secret [get]
func (c *ApiController) GetSecret() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	namespace := c.GetString("namespace")
	name := c.GetString("name")
	s, err := object.GetSecret(cfg, namespace, name)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(toSecretDetail(*s))
}

type secretRequest struct {
	Namespace       string            `json:"namespace"`
	Name            string            `json:"name"`
	Type            string            `json:"type"`
	StringData      map[string]string `json:"stringData"`
	ResourceVersion string            `json:"resourceVersion"`
}

// AddSecret
// @router /api/add-secret [post]
func (c *ApiController) AddSecret() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	var req secretRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}
	if req.Namespace == "" {
		req.Namespace = "default"
	}
	secretType := corev1.SecretTypeOpaque
	if req.Type != "" {
		secretType = corev1.SecretType(req.Type)
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.Name,
			Namespace: req.Namespace,
		},
		Type:       secretType,
		StringData: req.StringData,
	}
	created, err := object.AddSecret(cfg, secret)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(toSecretDetail(*created))
}

// UpdateSecret
// @router /api/update-secret [post]
func (c *ApiController) UpdateSecret() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	var req secretRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}
	if req.Namespace == "" {
		req.Namespace = "default"
	}
	secretType := corev1.SecretTypeOpaque
	if req.Type != "" {
		secretType = corev1.SecretType(req.Type)
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:            req.Name,
			Namespace:       req.Namespace,
			ResourceVersion: req.ResourceVersion,
		},
		Type:       secretType,
		StringData: req.StringData,
	}
	updated, err := object.UpdateSecret(cfg, secret)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(toSecretDetail(*updated))
}

// DeleteSecret
// @router /api/delete-secret [post]
func (c *ApiController) DeleteSecret() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	var req secretRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}
	if req.Namespace == "" {
		req.Namespace = "default"
	}
	if err := object.DeleteSecret(cfg, req.Namespace, req.Name); err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk()
}
