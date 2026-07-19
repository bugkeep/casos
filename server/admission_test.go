package server

import (
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	admissionv1 "k8s.io/api/admission/v1"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/kubernetes/plugin/pkg/auth/authorizer/rbac/bootstrappolicy"
)

func TestGenericSystemIdentityDoesNotBypassAdmission(t *testing.T) {
	req := &admissionv1.AdmissionRequest{
		Resource:  metav1.GroupVersionResource{Resource: "pods"},
		Operation: admissionv1.Create,
		UserInfo: authenticationv1.UserInfo{
			Username: "system:serviceaccount:default:installer",
			Groups:   []string{"system:serviceaccounts", "system:authenticated"},
		},
	}
	if !shouldEnforceAdmissionPolicy(req) {
		t.Fatal("a generic authenticated service account bypassed admission policy")
	}
}

func TestTrustedControlPlaneIdentitiesBypassCasbin(t *testing.T) {
	for _, userInfo := range []authenticationv1.UserInfo{
		{Username: "system:kube-controller-manager"},
		{Username: "system:kube-scheduler"},
		{Username: "system:node:worker-1", Groups: []string{"system:nodes", "system:authenticated"}},
	} {
		req := &admissionv1.AdmissionRequest{
			Resource: metav1.GroupVersionResource{Resource: "pods"},
			UserInfo: userInfo,
		}
		if shouldEnforceAdmissionPolicy(req) {
			t.Fatalf("trusted control-plane identity %q was subject to end-user Casbin policy", userInfo.Username)
		}
	}
}

func TestPlatformPodLabelDoesNotAuthenticateCaller(t *testing.T) {
	pod := corev1.Pod{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
		"app.kubernetes.io/managed-by": "casos",
	}}}
	raw, err := json.Marshal(&pod)
	if err != nil {
		t.Fatalf("encode Pod: %v", err)
	}
	req := &admissionv1.AdmissionRequest{
		Namespace: "kube-system",
		Resource:  metav1.GroupVersionResource{Resource: "pods"},
		Operation: admissionv1.Create,
		Object:    runtime.RawExtension{Raw: raw},
		UserInfo:  authenticationv1.UserInfo{Username: "alice"},
	}
	if !shouldEnforceAdmissionPolicy(req) || !shouldCheckPodImages(req) {
		t.Fatal("a user-controlled platform label bypassed admission")
	}
}

func TestForgedOwnerReferenceDoesNotBypassPodAdmission(t *testing.T) {
	controller := true
	pod := corev1.Pod{ObjectMeta: metav1.ObjectMeta{OwnerReferences: []metav1.OwnerReference{{
		APIVersion: "apps/v1", Kind: "ReplicaSet", Name: "forged", Controller: &controller,
	}}}}
	raw, err := json.Marshal(&pod)
	if err != nil {
		t.Fatalf("encode Pod: %v", err)
	}
	req := &admissionv1.AdmissionRequest{
		Resource:  metav1.GroupVersionResource{Resource: "pods"},
		Operation: admissionv1.Create,
		Object:    runtime.RawExtension{Raw: raw},
		UserInfo:  authenticationv1.UserInfo{Username: "alice"},
	}
	if isWorkloadControllerPodRequest(req) {
		t.Fatal("a user-controlled ownerReference bypassed admission")
	}
	if !shouldEnforceAdmissionPolicy(req) || !shouldCheckPodImages(req) {
		t.Fatal("forged Pod did not remain subject to policy and image checks")
	}
}

func TestReplicaSetTemplateIsImageChecked(t *testing.T) {
	req := &admissionv1.AdmissionRequest{
		Namespace: "default",
		Resource:  metav1.GroupVersionResource{Group: "apps", Resource: "replicasets"},
		Operation: admissionv1.Create,
	}
	if !shouldCheckWorkloadTemplateImages(req) {
		t.Fatal("ReplicaSet templates must be checked before controller-created Pods are exempted")
	}
}

func TestWorkloadTemplateChecksOnlyBuiltInAPIGroups(t *testing.T) {
	for _, tc := range []struct {
		group, resource string
		want            bool
	}{
		{group: "apps", resource: "deployments", want: true},
		{group: "batch", resource: "jobs", want: true},
		{group: "", resource: "replicationcontrollers", want: true},
		{group: "example.com", resource: "deployments", want: false},
		{group: "apps", resource: "jobs", want: false},
	} {
		req := &admissionv1.AdmissionRequest{
			Namespace: "default",
			Resource:  metav1.GroupVersionResource{Group: tc.group, Resource: tc.resource},
			Operation: admissionv1.Create,
		}
		if got := shouldCheckWorkloadTemplateImages(req); got != tc.want {
			t.Fatalf("group=%q resource=%q: got %v, want %v", tc.group, tc.resource, got, tc.want)
		}
	}
}

func TestControllerDerivedWorkloadIsNotRechecked(t *testing.T) {
	req := &admissionv1.AdmissionRequest{
		Namespace: "default",
		Resource:  metav1.GroupVersionResource{Group: "apps", Resource: "replicasets"},
		Operation: admissionv1.Create,
		UserInfo:  authenticationv1.UserInfo{Username: "system:kube-controller-manager"},
	}
	if shouldEnforceAdmissionPolicy(req) {
		t.Fatal("controller-derived ReplicaSet was subject to end-user Casbin policy")
	}
	if shouldCheckWorkloadTemplateImages(req) {
		t.Fatal("controller-derived ReplicaSet rechecked an already-approved Pod template")
	}
}

func TestMalformedWorkloadsFailImageParsing(t *testing.T) {
	if message := checkPodImages([]byte(`{"spec":`), false); message == "" {
		t.Fatal("malformed Pod bypassed image checks")
	}
	for resource, location := range workloadPodSpecLocations {
		if message := checkWorkloadTemplateImages(location.group, resource, []byte(`{"spec":`), false); message == "" {
			t.Fatalf("malformed %s bypassed image checks", resource)
		}
	}
}

func TestDecodeWorkloadPodSpecIsVersionAgnostic(t *testing.T) {
	raw := []byte(`{
		"apiVersion":"extensions/v1beta1",
		"kind":"Deployment",
		"spec":{"template":{"spec":{"terminationGracePeriodSeconds":30,"containers":[{"name":"app","image":"example/app:v1"}]}}}
	}`)
	path, ok := workloadPodSpecPath("apps", "deployments")
	if !ok {
		t.Fatal("Deployment Pod spec path is missing")
	}
	spec, found, err := decodePodSpec(raw, path...)
	if err != nil || !found {
		t.Fatalf("decode version-skewed workload: found=%v err=%v", found, err)
	}
	if len(spec.Containers) != 1 || spec.Containers[0].Image != "example/app:v1" {
		t.Fatalf("unexpected Pod spec: %#v", spec)
	}
	if spec.TerminationGracePeriodSeconds == nil || *spec.TerminationGracePeriodSeconds != 30 {
		t.Fatalf("integer field was not preserved: %#v", spec.TerminationGracePeriodSeconds)
	}
}

func TestCasbinAdmissionWebhookRemainsRecoverable(t *testing.T) {
	webhook := casbinAdmissionWebhook("https://127.0.0.1:9443/admission/validate", []byte("ca"))
	if webhook.FailurePolicy == nil || *webhook.FailurePolicy != admissionregv1.Ignore {
		t.Fatalf("expected fail-open recovery policy, got %v", webhook.FailurePolicy)
	}
	if len(webhook.Rules) != 1 || webhook.Rules[0].Rule.Scope == nil || *webhook.Rules[0].Rule.Scope != admissionregv1.AllScopes {
		t.Fatalf("unexpected webhook scope: %#v", webhook.Rules)
	}
	if webhook.TimeoutSeconds == nil || *webhook.TimeoutSeconds != 3 {
		t.Fatalf("expected bounded webhook timeout, got %v", webhook.TimeoutSeconds)
	}
	if webhook.SideEffects == nil || *webhook.SideEffects != admissionregv1.SideEffectClassNoneOnDryRun {
		t.Fatalf("unexpected side-effect declaration: %v", webhook.SideEffects)
	}
}

func TestDryRunDoesNotTriggerImageScans(t *testing.T) {
	dryRun := true
	if shouldTriggerImageScans(&admissionv1.AdmissionRequest{DryRun: &dryRun}) {
		t.Fatal("dry-run admission was allowed to trigger background scans")
	}
	dryRun = false
	if !shouldTriggerImageScans(&admissionv1.AdmissionRequest{DryRun: &dryRun}) {
		t.Fatal("normal admission did not allow background scans")
	}
}

func TestPlatformBootstrapExemptionRequiresGeneratedIdentity(t *testing.T) {
	base := admissionv1.AdmissionRequest{
		Namespace: "kube-system",
		Resource:  metav1.GroupVersionResource{Group: "apps", Resource: "deployments"},
		Operation: admissionv1.Create,
	}
	for _, tc := range []struct {
		name     string
		userInfo authenticationv1.UserInfo
		want     bool
	}{
		{name: "generated admin", userInfo: authenticationv1.UserInfo{Username: "admin", Groups: []string{"system:masters"}}, want: false},
		{name: "name only", userInfo: authenticationv1.UserInfo{Username: "admin"}, want: true},
		{name: "group only", userInfo: authenticationv1.UserInfo{Username: "other", Groups: []string{"system:masters"}}, want: true},
	} {
		req := base
		req.UserInfo = tc.userInfo
		if got := shouldCheckWorkloadTemplateImages(&req); got != tc.want {
			t.Fatalf("%s: got imageCheck=%v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestComponentKubeconfigUsesDedicatedIdentity(t *testing.T) {
	dir := t.TempDir()
	if err := ensureCerts(dir, "127.0.0.1", "127.0.0.1"); err != nil {
		t.Fatalf("ensure certificates: %v", err)
	}
	path, err := ensureComponentKubeconfig(dir, "https://127.0.0.1:6443", "controller-manager")
	if err != nil {
		t.Fatalf("ensure component kubeconfig: %v", err)
	}
	config, err := clientcmd.LoadFromFile(path)
	if err != nil {
		t.Fatalf("load kubeconfig: %v", err)
	}
	auth := config.AuthInfos["controller-manager"]
	if auth == nil {
		t.Fatal("controller-manager auth info is missing")
	}
	block, _ := pem.Decode(auth.ClientCertificateData)
	if block == nil {
		t.Fatal("component client certificate is not PEM")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parse component client certificate: %v", err)
	}
	if cert.Subject.CommonName != "system:kube-controller-manager" {
		t.Fatalf("component common name = %q", cert.Subject.CommonName)
	}

	caCertPEM, err := os.ReadFile(filepath.Join(dir, "ca.crt"))
	if err != nil {
		t.Fatalf("read CA certificate: %v", err)
	}
	caBlock, _ := pem.Decode(caCertPEM)
	caCert, err := x509.ParseCertificate(caBlock.Bytes)
	if err != nil {
		t.Fatalf("parse CA certificate: %v", err)
	}
	caKeyPEM, err := os.ReadFile(filepath.Join(dir, "ca.key"))
	if err != nil {
		t.Fatalf("read CA key: %v", err)
	}
	caKeyBlock, _ := pem.Decode(caKeyPEM)
	caKey, err := x509.ParsePKCS1PrivateKey(caKeyBlock.Bytes)
	if err != nil {
		t.Fatalf("parse CA key: %v", err)
	}
	componentKeyBlock, _ := pem.Decode(auth.ClientKeyData)
	componentKey, err := x509.ParseECPrivateKey(componentKeyBlock.Bytes)
	if err != nil {
		t.Fatalf("parse component key: %v", err)
	}
	expiredTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(99),
		Subject:      pkix.Name{CommonName: "system:kube-controller-manager"},
		NotBefore:    time.Now().Add(-2 * time.Hour),
		NotAfter:     time.Now().Add(-time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	expiredDER, err := x509.CreateCertificate(rand.Reader, expiredTemplate, caCert, &componentKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create expired component certificate: %v", err)
	}
	auth.ClientCertificateData = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: expiredDER})
	if err := clientcmd.WriteToFile(*config, path); err != nil {
		t.Fatalf("write expired component kubeconfig: %v", err)
	}
	if _, err := ensureComponentKubeconfig(dir, "https://127.0.0.1:6443", "controller-manager"); err != nil {
		t.Fatalf("renew expired component certificate: %v", err)
	}
	renewedConfig, err := clientcmd.LoadFromFile(path)
	if err != nil {
		t.Fatalf("load renewed component kubeconfig: %v", err)
	}
	renewedBlock, _ := pem.Decode(renewedConfig.AuthInfos["controller-manager"].ClientCertificateData)
	renewed, err := x509.ParseCertificate(renewedBlock.Bytes)
	if err != nil {
		t.Fatalf("parse renewed component certificate: %v", err)
	}
	if !renewed.NotAfter.After(time.Now()) {
		t.Fatal("expired component certificate was reused")
	}

	// A legacy kubeconfig using the admin certificate must be replaced on the
	// next startup, not retained indefinitely by the old write-once behavior.
	if err := os.WriteFile(path, []byte("legacy"), 0o600); err != nil {
		t.Fatalf("write legacy kubeconfig: %v", err)
	}
	if _, err := ensureComponentKubeconfig(dir, "https://127.0.0.1:6443", "controller-manager"); err != nil {
		t.Fatalf("rewrite component kubeconfig: %v", err)
	}
	if data, err := os.ReadFile(filepath.Clean(path)); err != nil || string(data) == "legacy" {
		t.Fatalf("legacy component kubeconfig was not replaced: %v", err)
	}
}

func TestComponentIdentitiesHaveKubernetesBootstrapBindings(t *testing.T) {
	want := map[string]bool{
		"system:kube-controller-manager": false,
		"system:kube-scheduler":          false,
	}
	for _, binding := range bootstrappolicy.ClusterRoleBindings() {
		for _, subject := range binding.Subjects {
			if _, ok := want[subject.Name]; ok {
				want[subject.Name] = true
			}
		}
	}
	for identity, found := range want {
		if !found {
			t.Fatalf("Kubernetes bootstrap RBAC does not bind %s", identity)
		}
	}
}
