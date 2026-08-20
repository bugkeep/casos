package server

import (
	"testing"
	"time"
)

func TestRecordAdmissionDenialAggregatesRetries(t *testing.T) {
	ClearAdmissionDenials()
	t.Cleanup(ClearAdmissionDenials)

	for i := 0; i < 3; i++ {
		RecordAdmissionDenial("system:node:wsl-ubuntu", "kube-node-lease", "leases", "UPDATE", "no allow rule")
	}

	found := AdmissionDenialsFor("system:node:wsl-ubuntu")
	if len(found) != 1 {
		t.Fatalf("AdmissionDenialsFor returned %d entries, want 1", len(found))
	}
	got := found[0]
	if got.Count != 3 {
		t.Errorf("Count = %d, want 3 (a retrying kubelet must not create three entries)", got.Count)
	}
	if got.Resource != "leases" || got.Operation != "UPDATE" {
		t.Errorf("got resource %q operation %q", got.Resource, got.Operation)
	}
	if len(ListAdmissionDenials()) != 1 {
		t.Errorf("ListAdmissionDenials returned %d entries, want 1", len(ListAdmissionDenials()))
	}
}

func TestAdmissionDenialsForPicksMostRecent(t *testing.T) {
	ClearAdmissionDenials()
	t.Cleanup(ClearAdmissionDenials)

	RecordAdmissionDenial("system:node:wsl-ubuntu", "kube-node-lease", "leases", "UPDATE", "older")
	time.Sleep(2 * time.Millisecond)
	RecordAdmissionDenial("system:node:wsl-ubuntu", "*", "nodes", "CREATE", "newer")

	found := AdmissionDenialsFor("system:node:wsl-ubuntu")
	if len(found) != 2 {
		t.Fatalf("got %d denials, want both", len(found))
	}
	if found[0].Resource != "nodes" {
		t.Fatalf("got %q first, want the newer nodes/CREATE denial", found[0].Resource)
	}
}

func TestAdmissionDenialsForIsScopedToSubject(t *testing.T) {
	ClearAdmissionDenials()
	t.Cleanup(ClearAdmissionDenials)

	RecordAdmissionDenial("system:node:other-worker", "*", "nodes", "CREATE", "not yours")

	if got := AdmissionDenialsFor("system:node:wsl-ubuntu"); len(got) != 0 {
		t.Errorf("got %+v for a subject that was never denied, want none", got)
	}
}

func TestAdmissionDenialsForReturnsACopy(t *testing.T) {
	ClearAdmissionDenials()
	t.Cleanup(ClearAdmissionDenials)

	RecordAdmissionDenial("system:node:wsl-ubuntu", "*", "nodes", "CREATE", "denied")
	AdmissionDenialsFor("system:node:wsl-ubuntu")[0].Count = 999
	RecordAdmissionDenial("system:node:wsl-ubuntu", "*", "nodes", "CREATE", "denied")

	if again := AdmissionDenialsFor("system:node:wsl-ubuntu")[0]; again.Count != 2 {
		t.Errorf("Count = %d, want 2 — the caller mutated the stored entry", again.Count)
	}
}

func TestClearAdmissionDenials(t *testing.T) {
	RecordAdmissionDenial("system:node:wsl-ubuntu", "*", "nodes", "CREATE", "denied")
	ClearAdmissionDenials()
	if got := AdmissionDenialsFor("system:node:wsl-ubuntu"); len(got) != 0 {
		t.Errorf("got %+v after clearing, want none", got)
	}
}

func TestRecordAdmissionDenialEvictsOldest(t *testing.T) {
	ClearAdmissionDenials()
	t.Cleanup(ClearAdmissionDenials)

	for i := 0; i < maxTrackedDenials+10; i++ {
		RecordAdmissionDenial("subject", "ns", "resource", string(rune('a'+i%26))+string(rune(i)), "denied")
	}
	if n := len(ListAdmissionDenials()); n > maxTrackedDenials {
		t.Errorf("stored %d denials, want at most %d", n, maxTrackedDenials)
	}
}
