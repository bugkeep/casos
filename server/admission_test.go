package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	admissionv1 "k8s.io/api/admission/v1"
	authenticationv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// denyAll stands in for a policy an operator narrowed to their own users: it
// matches nothing, which is exactly the state that locked kubelet out.
func denyAll(t *testing.T) *bool {
	t.Helper()
	called := new(bool)
	prev := enforceAdmission
	enforceAdmission = func(user, namespace, resource, action string) (bool, error) {
		*called = true
		return false, nil
	}
	t.Cleanup(func() { enforceAdmission = prev })
	return called
}

func postReview(t *testing.T, user string, groups []string, resource, namespace string, op admissionv1.Operation) *admissionv1.AdmissionResponse {
	t.Helper()
	review := admissionv1.AdmissionReview{
		Request: &admissionv1.AdmissionRequest{
			UID:       "uid-1",
			Namespace: namespace,
			Operation: op,
			Resource:  metav1.GroupVersionResource{Resource: resource},
			UserInfo:  authenticationv1.UserInfo{Username: user, Groups: groups},
		},
	}
	body, err := json.Marshal(review)
	if err != nil {
		t.Fatalf("marshal review: %v", err)
	}
	rec := httptest.NewRecorder()
	admissionValidateHandler(rec, httptest.NewRequest(http.MethodPost, "/admission/validate", bytes.NewReader(body)))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var out admissionv1.AdmissionReview
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.Response == nil {
		t.Fatal("response has no AdmissionResponse")
	}
	return out.Response
}

func TestAdmissionAllowsKubeletUnderADenyAllPolicy(t *testing.T) {
	ClearAdmissionDenials()
	t.Cleanup(ClearAdmissionDenials)
	called := denyAll(t)

	resp := postReview(t, "system:node:wsl-ubuntu", []string{"system:nodes", "system:authenticated"}, "nodes", "", admissionv1.Create)

	if !resp.Allowed {
		t.Fatalf("kubelet node registration denied: %v — the node never goes Ready", resp.Result)
	}
	if *called {
		t.Error("the operator's policy was evaluated for a kubelet; system: subjects are not in it")
	}
	if got := AdmissionDenialsFor("system:node:wsl-ubuntu"); len(got) != 0 {
		t.Errorf("recorded %+v for an allowed request", got)
	}
}

func TestAdmissionAllowsControlPlaneAndControllers(t *testing.T) {
	denyAll(t)
	t.Cleanup(ClearAdmissionDenials)

	cases := []struct {
		name   string
		user   string
		groups []string
	}{
		{"kubelet lease", "system:node:wsl-ubuntu", []string{"system:nodes"}},
		{"controller manager", "system:kube-controller-manager", nil},
		{"replicaset controller", "system:serviceaccount:kube-system:replicaset-controller", []string{"system:serviceaccounts"}},
		{"admin cert", "kubernetes-admin", []string{"system:masters", "system:authenticated"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if resp := postReview(t, tc.user, tc.groups, "leases", "kube-node-lease", admissionv1.Update); !resp.Allowed {
				t.Errorf("%s denied: %v", tc.user, resp.Result)
			}
		})
	}
}

func TestAdmissionStillEnforcesForRegularUsers(t *testing.T) {
	ClearAdmissionDenials()
	t.Cleanup(ClearAdmissionDenials)
	called := denyAll(t)

	// system:authenticated rides along on every request, so it must not buy an
	// exemption — otherwise the policy would enforce nothing at all.
	resp := postReview(t, "alice", []string{"system:authenticated", "developers"}, "pods", "default", admissionv1.Create)

	if resp.Allowed {
		t.Fatal("alice was allowed under a deny-all policy")
	}
	if !*called {
		t.Error("the policy was never evaluated for a regular user")
	}
	denied := AdmissionDenialsFor("alice")
	if len(denied) != 1 {
		t.Fatalf("recorded %d denials for alice, want 1", len(denied))
	}
	if denied[0].Resource != "pods" || denied[0].Namespace != "default" {
		t.Errorf("recorded %+v, want the pods/default request", denied[0])
	}
}

func TestAdmissionReportsPolicyEvaluationErrors(t *testing.T) {
	ClearAdmissionDenials()
	t.Cleanup(ClearAdmissionDenials)
	prev := enforceAdmission
	enforceAdmission = func(string, string, string, string) (bool, error) {
		return false, errors.New("boom")
	}
	t.Cleanup(func() { enforceAdmission = prev })

	resp := postReview(t, "alice", nil, "pods", "default", admissionv1.Create)

	if resp.Allowed {
		t.Fatal("allowed despite an unevaluable policy")
	}
	if resp.Result == nil || !bytes.Contains([]byte(resp.Result.Message), []byte("boom")) {
		t.Errorf("message = %v, want the underlying error", resp.Result)
	}
}

func TestIsReservedIdentity(t *testing.T) {
	cases := []struct {
		user   string
		groups []string
		want   bool
	}{
		{"system:node:wsl-ubuntu", []string{"system:nodes"}, true},
		{"system:serviceaccount:default:build", nil, true},
		{"alice", []string{"system:authenticated"}, false},
		{"alice", []string{"system:unauthenticated"}, false},
		{"alice", []string{"developers"}, false},
		{"alice", []string{"system:masters"}, true},
	}
	for _, tc := range cases {
		if got := isReservedIdentity(tc.user, tc.groups); got != tc.want {
			t.Errorf("isReservedIdentity(%q, %v) = %v, want %v", tc.user, tc.groups, got, tc.want)
		}
	}
}
