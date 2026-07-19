package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/beego/beego/logs"
	"github.com/casosorg/casos/object"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

const admissionValidatePath = "/admission/validate"

// RegisterAdmissionHandler mounts the ValidatingAdmissionWebhook endpoint on mux.
func RegisterAdmissionHandler(mux *http.ServeMux) {
	mux.HandleFunc(admissionValidatePath, admissionValidateHandler)
}

func admissionValidateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var review admissionv1.AdmissionReview
	if err := json.NewDecoder(r.Body).Decode(&review); err != nil {
		http.Error(w, "decode error: "+err.Error(), http.StatusBadRequest)
		return
	}

	req := review.Request
	if req == nil {
		writeAdmissionResponse(w, &admissionv1.AdmissionReview{
			TypeMeta: metav1.TypeMeta{APIVersion: "admission.k8s.io/v1", Kind: "AdmissionReview"},
			Response: &admissionv1.AdmissionResponse{
				Allowed: false,
				Result:  &metav1.Status{Message: "admission request is missing"},
			},
		})
		return
	}
	// Requests are allowed without a Casbin lookup only for the small set of
	// authenticated control-plane identities enumerated below.
	allowed := true
	var err error
	if shouldEnforceAdmissionPolicy(req) {
		allowed, err = object.EnforceAdmissionPolicy(
			req.UserInfo.Username,
			req.Namespace,
			req.Resource.Resource,
			string(req.Operation),
		)
	}

	resp := &admissionv1.AdmissionReview{
		TypeMeta: metav1.TypeMeta{APIVersion: "admission.k8s.io/v1", Kind: "AdmissionReview"},
		Response: &admissionv1.AdmissionResponse{
			UID:     req.UID,
			Allowed: allowed && err == nil,
		},
	}

	if !allowed || err != nil {
		msg := "denied by Casbin policy"
		if err != nil {
			msg = err.Error()
		}
		resp.Response.Result = &metav1.Status{Message: msg}
		writeAdmissionResponse(w, resp)
		return
	}

	// Check user-owned workload templates before they are accepted. Pods later
	// created by the workload controller inherit that already-approved template
	// and must not be blocked by a scan result that changed after installation.
	triggerUnknownScans := shouldTriggerImageScans(req)
	if shouldCheckWorkloadTemplateImages(req) {
		if denyMsg := checkWorkloadTemplateImages(req.Resource.Group, req.Resource.Resource, req.Object.Raw, triggerUnknownScans); denyMsg != "" {
			resp.Response.Allowed = false
			resp.Response.Result = &metav1.Status{Message: denyMsg}
			writeAdmissionResponse(w, resp)
			return
		}
	} else if shouldCheckPodImages(req) {
		if denyMsg := checkPodImages(req.Object.Raw, triggerUnknownScans); denyMsg != "" {
			resp.Response.Allowed = false
			resp.Response.Result = &metav1.Status{Message: denyMsg}
			writeAdmissionResponse(w, resp)
			return
		}
	}

	writeAdmissionResponse(w, resp)
}

func shouldTriggerImageScans(req *admissionv1.AdmissionRequest) bool {
	return req != nil && (req.DryRun == nil || !*req.DryRun)
}

func shouldEnforceAdmissionPolicy(req *admissionv1.AdmissionRequest) bool {
	if req == nil {
		return false
	}
	if isTrustedControlPlaneUser(req.UserInfo.Username, req.UserInfo.Groups) {
		return false
	}
	return true
}

func isTrustedControlPlaneUser(username string, groups []string) bool {
	// These identities are issued by the local CA or Kubernetes service-account
	// authenticator. Impersonating them requires explicit cluster-level RBAC,
	// which already grants enough authority to alter admission configuration.
	if isWorkloadControllerUser(username) || username == "system:kube-scheduler" {
		return true
	}
	if !strings.HasPrefix(username, "system:node:") {
		return false
	}
	for _, group := range groups {
		if group == "system:nodes" {
			return true
		}
	}
	return false
}

// Platform components must be able to restart and scale even when an image's
// cached Trivy result is stale or critical. Application namespaces remain
// subject to the normal image admission policy.
func isPlatformNamespace(namespace string) bool {
	switch namespace {
	case "kube-system", "kube-flannel", "local-path-storage":
		return true
	default:
		return false
	}
}

func shouldCheckPodImages(req *admissionv1.AdmissionRequest) bool {
	if req == nil || req.Resource.Resource != "pods" {
		return false
	}
	if req.SubResource != "" || isWorkloadControllerPodRequest(req) {
		return false
	}
	if req.Operation != admissionv1.Create && req.Operation != admissionv1.Update {
		return false
	}
	return true
}

func shouldCheckWorkloadTemplateImages(req *admissionv1.AdmissionRequest) bool {
	if req == nil || req.SubResource != "" {
		return false
	}
	if isWorkloadControllerUser(req.UserInfo.Username) {
		return false
	}
	if req.Operation != admissionv1.Create && req.Operation != admissionv1.Update {
		return false
	}
	if isPlatformNamespace(req.Namespace) && isPlatformBootstrapUser(req.UserInfo.Username, req.UserInfo.Groups) {
		return false
	}
	_, ok := workloadPodSpecPath(req.Resource.Group, req.Resource.Resource)
	return ok
}

type workloadPodSpecLocation struct {
	group string
	path  []string
}

var workloadPodSpecLocations = map[string]workloadPodSpecLocation{
	"deployments":            {group: "apps", path: []string{"spec", "template", "spec"}},
	"statefulsets":           {group: "apps", path: []string{"spec", "template", "spec"}},
	"daemonsets":             {group: "apps", path: []string{"spec", "template", "spec"}},
	"replicasets":            {group: "apps", path: []string{"spec", "template", "spec"}},
	"jobs":                   {group: "batch", path: []string{"spec", "template", "spec"}},
	"cronjobs":               {group: "batch", path: []string{"spec", "jobTemplate", "spec", "template", "spec"}},
	"replicationcontrollers": {group: "", path: []string{"spec", "template", "spec"}},
}

func workloadPodSpecPath(group, resource string) ([]string, bool) {
	location, ok := workloadPodSpecLocations[resource]
	if !ok || location.group != group {
		return nil, false
	}
	return location.path, true
}

func isPlatformBootstrapUser(username string, groups []string) bool {
	// AdminRestConfig is the only in-process bootstrap client and is issued this
	// exact user/group pair by ensureCerts. Requiring both avoids treating an
	// arbitrary account named admin as trusted.
	if username != "admin" {
		return false
	}
	for _, group := range groups {
		if group == "system:masters" {
			return true
		}
	}
	return false
}

func isWorkloadControllerPodRequest(req *admissionv1.AdmissionRequest) bool {
	if req == nil || req.Resource.Resource != "pods" {
		return false
	}
	if req.Operation != admissionv1.Create && req.Operation != admissionv1.Update && req.Operation != admissionv1.Delete {
		return false
	}
	return isWorkloadControllerUser(req.UserInfo.Username)
}

func isWorkloadControllerUser(username string) bool {
	const serviceAccountPrefix = "system:serviceaccount:kube-system:"
	if !strings.HasPrefix(username, serviceAccountPrefix) {
		return username == "system:kube-controller-manager"
	}
	switch strings.TrimPrefix(username, serviceAccountPrefix) {
	case "replicaset-controller", "replication-controller", "statefulset-controller", "daemon-set-controller", "job-controller", "cronjob-controller", "deployment-controller":
		return true
	default:
		return false
	}
}

// checkPodImages extracts images from the Pod spec, checks Trivy cache, and
// triggers async scans for unknown images. Returns a non-empty denial message
// if any image has CRITICAL vulnerabilities in the cache.
func checkPodImages(raw []byte, triggerUnknownScans bool) string {
	podSpec, found, err := decodePodSpec(raw, "spec")
	if err != nil {
		return fmt.Sprintf("failed to parse Pod for image policy: %v", err)
	}
	if !found {
		return "failed to parse Pod for image policy: spec is missing"
	}
	return checkPodSpecImages(podSpec, triggerUnknownScans)
}

// checkWorkloadTemplateImages intentionally fails closed on parse errors. The
// caller limits this function to built-in workload API groups, so malformed
// input must not bypass a security check.
func checkWorkloadTemplateImages(group, resource string, raw []byte, triggerUnknownScans bool) string {
	path, ok := workloadPodSpecPath(group, resource)
	if !ok {
		return ""
	}
	podSpec, found, err := decodePodSpec(raw, path...)
	if err != nil {
		return fmt.Sprintf("failed to parse %s for image policy: %v", resource, err)
	}
	if !found {
		if resource == "replicationcontrollers" {
			// ReplicationController intentionally permits a nil template; without
			// one it cannot create Pods, so there are no images to validate.
			return ""
		}
		return fmt.Sprintf("failed to parse %s for image policy: Pod template spec is missing", resource)
	}
	return checkPodSpecImages(podSpec, triggerUnknownScans)
}

func decodePodSpec(raw []byte, fields ...string) (corev1.PodSpec, bool, error) {
	var object map[string]interface{}
	if err := json.Unmarshal(raw, &object); err != nil {
		return corev1.PodSpec{}, false, err
	}
	specMap, found, err := unstructured.NestedMap(object, fields...)
	if err != nil || !found {
		return corev1.PodSpec{}, found, err
	}
	var podSpec corev1.PodSpec
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(specMap, &podSpec); err != nil {
		return corev1.PodSpec{}, false, err
	}
	return podSpec, true, nil
}

func checkPodSpecImages(spec corev1.PodSpec, triggerUnknownScans bool) string {
	var images []string
	for _, c := range spec.InitContainers {
		images = append(images, c.Image)
	}
	for _, c := range spec.Containers {
		images = append(images, c.Image)
	}
	for _, c := range spec.EphemeralContainers {
		images = append(images, c.Image)
	}

	for _, image := range images {
		result, err := object.GetTrivyScanResultByImage(image)
		if err != nil {
			logs.Error("trivy cache lookup %s: %v", image, err)
			continue
		}
		if result == nil {
			// Dry-run admission must remain side-effect free.
			if triggerUnknownScans {
				object.TriggerScan(image)
			}
			continue
		}
		if result.Status == "done" && result.Critical > 0 {
			return fmt.Sprintf("image %s has %d CRITICAL vulnerabilities — update the image or remove it from the scan results to override", image, result.Critical)
		}
	}
	return ""
}

func writeAdmissionResponse(w http.ResponseWriter, resp *admissionv1.AdmissionReview) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		logs.Error("admission response encode: %v", err)
	}
}
