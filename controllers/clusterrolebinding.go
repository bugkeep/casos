package controllers

import (
	"encoding/json"
	"fmt"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/casosorg/casos/object"
)

type subjectSummary struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

type clusterRoleBindingSummary struct {
	Name            string           `json:"name"`
	RoleRef         string           `json:"roleRef"`
	Subjects        []subjectSummary `json:"subjects"`
	CreatedAt       string           `json:"createdAt"`
	ResourceVersion string           `json:"resourceVersion"`
}

func toCrbSummary(crb rbacv1.ClusterRoleBinding) clusterRoleBindingSummary {
	subjects := make([]subjectSummary, 0, len(crb.Subjects))
	for _, s := range crb.Subjects {
		subjects = append(subjects, subjectSummary{
			Kind:      s.Kind,
			Name:      s.Name,
			Namespace: s.Namespace,
		})
	}
	return clusterRoleBindingSummary{
		Name:            crb.Name,
		RoleRef:         crb.RoleRef.Name,
		Subjects:        subjects,
		CreatedAt:       crb.CreationTimestamp.UTC().Format("2006-01-02 15:04:05"),
		ResourceVersion: crb.ResourceVersion,
	}
}

// GetClusterRoleBindings
// @router /api/get-clusterrolebindings [get]
func (c *ApiController) GetClusterRoleBindings() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	crbs, err := object.GetClusterRoleBindings(cfg)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	result := make([]clusterRoleBindingSummary, 0, len(crbs))
	for _, crb := range crbs {
		result = append(result, toCrbSummary(crb))
	}
	c.ResponseOk(result)
}

// GetClusterRoleBinding
// @router /api/get-clusterrolebinding [get]
func (c *ApiController) GetClusterRoleBinding() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	name := c.GetString("name")
	crb, err := object.GetClusterRoleBinding(cfg, name)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(toCrbSummary(*crb))
}

type subjectRequest struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

type clusterRoleBindingRequest struct {
	Name            string           `json:"name"`
	RoleRef         string           `json:"roleRef"`
	RoleRefKind     string           `json:"roleRefKind"`
	Subjects        []subjectRequest `json:"subjects"`
	ResourceVersion string           `json:"resourceVersion"`
}

func normalizeClusterRoleBindingRoleRefKind(roleRefKind string) (string, error) {
	if roleRefKind == "" {
		return "ClusterRole", nil
	}
	if roleRefKind != "ClusterRole" {
		return "", fmt.Errorf("ClusterRoleBinding only supports roleRefKind=ClusterRole")
	}
	return roleRefKind, nil
}

func buildCrb(req clusterRoleBindingRequest) *rbacv1.ClusterRoleBinding {
	subjects := make([]rbacv1.Subject, 0, len(req.Subjects))
	for _, s := range req.Subjects {
		subj := rbacv1.Subject{
			Kind: s.Kind,
			Name: s.Name,
		}
		// Namespace is only meaningful for ServiceAccount subjects.
		if s.Kind == "ServiceAccount" {
			subj.Namespace = s.Namespace
		}
		subjects = append(subjects, subj)
	}
	return &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: req.Name},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     req.RoleRefKind,
			Name:     req.RoleRef,
		},
		Subjects: subjects,
	}
}

// AddClusterRoleBinding
// @router /api/add-clusterrolebinding [post]
func (c *ApiController) AddClusterRoleBinding() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	var req clusterRoleBindingRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}
	roleRefKind, err := normalizeClusterRoleBindingRoleRefKind(req.RoleRefKind)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	req.RoleRefKind = roleRefKind
	crb := buildCrb(req)
	created, err := object.AddClusterRoleBinding(cfg, crb)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(toCrbSummary(*created))
}

// UpdateClusterRoleBinding replaces subjects; roleRef is immutable after creation.
// @router /api/update-clusterrolebinding [post]
func (c *ApiController) UpdateClusterRoleBinding() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	var req clusterRoleBindingRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}
	if req.RoleRefKind != "" {
		if _, err := normalizeClusterRoleBindingRoleRefKind(req.RoleRefKind); err != nil {
			c.ResponseError(err.Error())
			return
		}
	}
	// Fetch existing to preserve immutable roleRef.
	existing, err := object.GetClusterRoleBinding(cfg, req.Name)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	subjects := make([]rbacv1.Subject, 0, len(req.Subjects))
	for _, s := range req.Subjects {
		subj := rbacv1.Subject{Kind: s.Kind, Name: s.Name}
		if s.Kind == "ServiceAccount" {
			subj.Namespace = s.Namespace
		}
		subjects = append(subjects, subj)
	}
	existing.Subjects = subjects
	existing.ResourceVersion = req.ResourceVersion
	updated, err := object.UpdateClusterRoleBinding(cfg, existing)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(toCrbSummary(*updated))
}

// DeleteClusterRoleBinding
// @router /api/delete-clusterrolebinding [post]
func (c *ApiController) DeleteClusterRoleBinding() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	var req clusterRoleBindingRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}
	if err := object.DeleteClusterRoleBinding(cfg, req.Name); err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk()
}
