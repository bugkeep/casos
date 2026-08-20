package controllers

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// resourceRequest carries the container resource fields the workload forms
// expose. The fields are pointers so "absent" and "empty" stay distinct: a
// payload that omits a key leaves the running value alone, an empty string
// removes it. Without that split a quantity could never be cleared once set,
// because the form always sends the box it pre-filled from the live spec.
type resourceRequest struct {
	CpuRequest    *string `json:"cpuRequest"`
	MemoryRequest *string `json:"memoryRequest"`
	CpuLimit      *string `json:"cpuLimit"`
	MemoryLimit   *string `json:"memoryLimit"`
}

type resourceSummary struct {
	CpuRequest    string `json:"cpuRequest"`
	MemoryRequest string `json:"memoryRequest"`
	CpuLimit      string `json:"cpuLimit"`
	MemoryLimit   string `json:"memoryLimit"`
}

func applyResources(container *corev1.Container, req resourceRequest) error {
	fields := []struct {
		name  string
		value *string
		key   corev1.ResourceName
		list  *corev1.ResourceList
	}{
		{"CPU request", req.CpuRequest, corev1.ResourceCPU, &container.Resources.Requests},
		{"memory request", req.MemoryRequest, corev1.ResourceMemory, &container.Resources.Requests},
		{"CPU limit", req.CpuLimit, corev1.ResourceCPU, &container.Resources.Limits},
		{"memory limit", req.MemoryLimit, corev1.ResourceMemory, &container.Resources.Limits},
	}

	for _, f := range fields {
		if f.value == nil {
			continue
		}
		text := strings.TrimSpace(*f.value)
		if text == "" {
			delete(*f.list, f.key)
			continue
		}
		q, err := resource.ParseQuantity(text)
		if err != nil {
			return fmt.Errorf("%s %q is not a valid quantity - use a value like 100m, 0.5, 256Mi or 1Gi", f.name, text)
		}
		if q.Sign() < 0 {
			return fmt.Errorf("%s %q must not be negative", f.name, text)
		}
		if *f.list == nil {
			*f.list = corev1.ResourceList{}
		}
		(*f.list)[f.key] = q
	}

	// The apiserver rejects this too, but only after a round trip and with the
	// whole object quoted back at the user.
	for _, key := range []corev1.ResourceName{corev1.ResourceCPU, corev1.ResourceMemory} {
		request, hasRequest := container.Resources.Requests[key]
		limit, hasLimit := container.Resources.Limits[key]
		if hasRequest && hasLimit && request.Cmp(limit) > 0 {
			return fmt.Errorf("%s limit %s must be at least the request %s", key, limit.String(), request.String())
		}
	}
	return nil
}

func extractResources(containers []corev1.Container) resourceSummary {
	if len(containers) == 0 {
		return resourceSummary{}
	}
	r := containers[0].Resources
	return resourceSummary{
		CpuRequest:    quantityString(r.Requests, corev1.ResourceCPU),
		MemoryRequest: quantityString(r.Requests, corev1.ResourceMemory),
		CpuLimit:      quantityString(r.Limits, corev1.ResourceCPU),
		MemoryLimit:   quantityString(r.Limits, corev1.ResourceMemory),
	}
}

func quantityString(list corev1.ResourceList, key corev1.ResourceName) string {
	if q, ok := list[key]; ok {
		return q.String()
	}
	return ""
}
