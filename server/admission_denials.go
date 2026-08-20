package server

import (
	"sort"
	"sync"
	"time"
)

// AdmissionDenial is one admission request the Casbin webhook rejected.
//
// A component that CasOS keeps rejecting looks, from the Kubernetes API's side,
// exactly like a component that died: the objects it should have written simply
// never appear. kubelet is the painful case — once its node status and lease
// updates are denied, the node goes NotReady with "Kubelet stopped posting node
// status", and the real reason exists only in the kubelet log on the node. So
// the webhook keeps its own denials here, and the UI joins them back onto the
// resources they broke.
type AdmissionDenial struct {
	Subject   string    `json:"subject"`
	Namespace string    `json:"namespace"`
	Resource  string    `json:"resource"`
	Operation string    `json:"operation"`
	Message   string    `json:"message"`
	Count     int       `json:"count"`
	FirstSeen time.Time `json:"firstSeen"`
	LastSeen  time.Time `json:"lastSeen"`
}

// A denied kubelet retries every few seconds, so denials are aggregated per
// (subject, namespace, resource, operation) rather than appended one per event.
// The cap bounds a cluster that is denying everything; the oldest entries go
// first because the recent ones are the ones explaining what is broken now.
const maxTrackedDenials = 512

var (
	denialMu sync.Mutex
	denials  = map[string]*AdmissionDenial{}
)

func denialKey(subject, namespace, resource, operation string) string {
	return subject + "\x00" + namespace + "\x00" + resource + "\x00" + operation
}

// RecordAdmissionDenial files one rejected request under its subject. The
// admission webhook is its only production caller.
func RecordAdmissionDenial(subject, namespace, resource, operation, message string) {
	if subject == "" {
		return
	}
	now := time.Now()

	denialMu.Lock()
	defer denialMu.Unlock()

	key := denialKey(subject, namespace, resource, operation)
	if d, ok := denials[key]; ok {
		d.Message = message
		d.Count++
		d.LastSeen = now
		return
	}
	if len(denials) >= maxTrackedDenials {
		evictOldestDenialLocked()
	}
	denials[key] = &AdmissionDenial{
		Subject:   subject,
		Namespace: namespace,
		Resource:  resource,
		Operation: operation,
		Message:   message,
		Count:     1,
		FirstSeen: now,
		LastSeen:  now,
	}
}

func evictOldestDenialLocked() {
	var oldestKey string
	var oldest time.Time
	for k, d := range denials {
		if oldestKey == "" || d.LastSeen.Before(oldest) {
			oldestKey, oldest = k, d.LastSeen
		}
	}
	delete(denials, oldestKey)
}

// AdmissionDenialsFor returns every denial recorded for subject, most recent
// first. A blocked kubelet is denied on several resources at once — its lease,
// its node status, its events — and naming only one of them would send the
// operator after a single symptom of a policy that is rejecting all of them.
// Callers get copies: the stored entries keep mutating as the subject retries.
func AdmissionDenialsFor(subject string) []AdmissionDenial {
	denialMu.Lock()
	defer denialMu.Unlock()

	out := []AdmissionDenial{}
	for _, d := range denials {
		if d.Subject == subject {
			out = append(out, *d)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LastSeen.After(out[j].LastSeen) })
	return out
}

// ListAdmissionDenials returns every recorded denial, most recent first.
func ListAdmissionDenials() []AdmissionDenial {
	denialMu.Lock()
	defer denialMu.Unlock()

	out := make([]AdmissionDenial, 0, len(denials))
	for _, d := range denials {
		out = append(out, *d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LastSeen.After(out[j].LastSeen) })
	return out
}

// ClearAdmissionDenials drops the recorded denials. The policy that caused them
// is gone once it is edited, so keeping stale entries would keep explaining a
// failure that no longer exists.
func ClearAdmissionDenials() {
	denialMu.Lock()
	defer denialMu.Unlock()
	denials = map[string]*AdmissionDenial{}
}
