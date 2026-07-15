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
)

// RegisterAdmissionHandler mounts the ValidatingAdmissionWebhook endpoint on mux.
func RegisterAdmissionHandler(mux *http.ServeMux) {
	mux.HandleFunc("/admission/validate", admissionValidateHandler)
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
	platformRequest := isPlatformPodRequest(req)
	controllerPodRequest := isWorkloadControllerPodRequest(req)
	allowed, err := true, error(nil)
	if !platformRequest && !controllerPodRequest {
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

	// Image vulnerability check: only for Pod-creating operations.
	if req.Resource.Resource == "pods" && !platformRequest && !controllerPodRequest && (req.Operation == admissionv1.Create || req.Operation == admissionv1.Update) {
		if denyMsg := checkPodImages(req.Object.Raw); denyMsg != "" {
			resp.Response.Allowed = false
			resp.Response.Result = &metav1.Status{Message: denyMsg}
			writeAdmissionResponse(w, resp)
			return
		}
	}

	writeAdmissionResponse(w, resp)
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

func isPlatformPodRequest(req *admissionv1.AdmissionRequest) bool {
	if req == nil || req.Resource.Resource != "pods" {
		return false
	}
	if req.Operation != admissionv1.Create && req.Operation != admissionv1.Update {
		return false
	}
	return isPlatformNamespace(req.Namespace)
}

func isWorkloadControllerPodRequest(req *admissionv1.AdmissionRequest) bool {
	if req == nil || req.Resource.Resource != "pods" {
		return false
	}
	if req.Operation != admissionv1.Create && req.Operation != admissionv1.Update {
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
func checkPodImages(raw []byte) string {
	var pod corev1.Pod
	if err := json.Unmarshal(raw, &pod); err != nil {
		return ""
	}

	var images []string
	for _, c := range pod.Spec.InitContainers {
		images = append(images, c.Image)
	}
	for _, c := range pod.Spec.Containers {
		images = append(images, c.Image)
	}

	for _, image := range images {
		result, err := object.GetTrivyScanResultByImage(image)
		if err != nil {
			logs.Error("trivy cache lookup %s: %v", image, err)
			continue
		}
		if result == nil {
			// No cache yet — allow this time and kick off a background scan.
			object.TriggerScan(image)
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
