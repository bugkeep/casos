package server

import (
	"regexp"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestServiceLBDaemonSetName(t *testing.T) {
	tests := []struct {
		name    string
		service *corev1.Service
		want    string
	}{
		{
			name: "uses uid for the stable suffix",
			service: &corev1.Service{ObjectMeta: metav1.ObjectMeta{
				Name: "api",
				UID:  types.UID("uid-1"),
			}},
			want: "svclb-api-4a49acf8a6bd",
		},
		{
			name: "uses namespace and name before the api assigns a uid",
			service: &corev1.Service{ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "api",
			}},
			want: "svclb-api-d53b356d3e1e",
		},
		{
			name: "trims boundary hyphens",
			service: &corev1.Service{ObjectMeta: metav1.ObjectMeta{
				Name: "---",
			}},
			want: "svclb-service-ff24f66688f3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := serviceLBDaemonSetName(tt.service); got != tt.want {
				t.Fatalf("serviceLBDaemonSetName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestServiceLBDaemonSetNameFitsDNSLabel(t *testing.T) {
	service := &corev1.Service{ObjectMeta: metav1.ObjectMeta{
		Namespace: "default",
		Name:      strings.Repeat("a", 43) + "-" + strings.Repeat("b", 19),
	}}

	got := serviceLBDaemonSetName(service)
	if len(got) > 63 {
		t.Fatalf("serviceLBDaemonSetName() length = %d, want at most 63: %q", len(got), got)
	}
	if matched := regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`).MatchString(got); !matched {
		t.Fatalf("serviceLBDaemonSetName() = %q, want a DNS label", got)
	}
}

func TestServiceLBDaemonSetNameSeparatesNamespacesWithoutUID(t *testing.T) {
	first := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "api"}}
	second := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Namespace: "other", Name: "api"}}

	firstName := serviceLBDaemonSetName(first)
	secondName := serviceLBDaemonSetName(second)
	if firstName == secondName {
		t.Fatalf("serviceLBDaemonSetName() reused %q across namespaces", firstName)
	}
	if secondName != "svclb-api-25dcc26e6d02" {
		t.Fatalf("serviceLBDaemonSetName() = %q, want %q", secondName, "svclb-api-25dcc26e6d02")
	}
}
