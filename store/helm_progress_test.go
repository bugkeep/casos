package store

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestHelmLogThrottleDropsRepeatsInsideWindow(t *testing.T) {
	now := time.Now()
	throttle := newHelmLogThrottle()
	throttle.now = func() time.Time { return now }

	line := "Deployment is not ready: default/kubeview. 0 out of 1 expected pods are ready"
	if !throttle.allow(line) {
		t.Fatal("first occurrence must be sent")
	}
	now = now.Add(2 * time.Second)
	if throttle.allow(line) {
		t.Fatal("a repeat inside the window must be dropped")
	}
	if !throttle.allow("creating 5 resource(s)") {
		t.Fatal("a different line must be sent")
	}
	now = now.Add(helmLogRepeatWindow)
	if !throttle.allow(line) {
		t.Fatal("a repeat after the window must be sent again")
	}
}

func TestHelmLogThrottleAllowsEmptyAndPrunes(t *testing.T) {
	now := time.Now()
	throttle := newHelmLogThrottle()
	throttle.now = func() time.Time { return now }

	if !throttle.allow("") {
		t.Fatal("an empty message must not be throttled")
	}
	for i := 0; i < helmLogRepeatMaxEntries+10; i++ {
		throttle.allow(strings.Repeat("x", i%64) + string(rune('a'+i%26)) + time.Duration(i).String())
	}
	if len(throttle.sentAt) > helmLogRepeatMaxEntries {
		t.Fatalf("throttle kept %d entries, want at most %d", len(throttle.sentAt), helmLogRepeatMaxEntries)
	}
}

func TestHelmProgressReporterReportsOnlyWhatChanged(t *testing.T) {
	var (
		mu       sync.Mutex
		reported []string
		round    int
	)
	diagnose := func() []string {
		mu.Lock()
		defer mu.Unlock()
		round++
		lines := []string{
			"Helm release diagnostics for default/kubeview:",
			"  selector: app.kubernetes.io/instance=kubeview",
			"  Pod kubeview-1: phase=Pending ready=0/1",
		}
		if round > 1 {
			lines = append(lines, "    container kubeview: state=waiting reason=ImagePullBackOff")
		}
		return lines
	}
	report := func(line string) {
		mu.Lock()
		defer mu.Unlock()
		reported = append(reported, line)
	}

	reporter := &helmProgressReporter{stop: make(chan struct{}), done: make(chan struct{})}
	go reporter.run(context.Background(), time.Millisecond, "kubeview", "default", diagnose, report)
	waitForReportedLine(t, &mu, &reported, "    container kubeview: state=waiting reason=ImagePullBackOff")
	reporter.Stop()

	mu.Lock()
	defer mu.Unlock()
	headings, pods, containers := 0, 0, 0
	for _, line := range reported {
		switch {
		case isHelmDiagnosticsHeading(line):
			headings++
		case line == "  Pod kubeview-1: phase=Pending ready=0/1":
			pods++
		case strings.HasPrefix(line, "    container kubeview:"):
			containers++
		case !strings.HasPrefix(line, "still waiting for default/kubeview"):
			t.Fatalf("unexpected reported line %q", line)
		}
	}
	if headings != 0 {
		t.Fatalf("reported %d diagnostics heading lines, want none", headings)
	}
	if pods != 1 {
		t.Fatalf("reported the unchanged Pod line %d times, want once", pods)
	}
	if containers != 1 {
		t.Fatalf("reported the new container line %d times, want once", containers)
	}
}

func TestHelmProgressReporterStopsWithContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	reporter := &helmProgressReporter{stop: make(chan struct{}), done: make(chan struct{})}
	go reporter.run(ctx, time.Millisecond, "kubeview", "default", func() []string { return nil }, func(string) {})
	cancel()

	select {
	case <-reporter.done:
	case <-time.After(5 * time.Second):
		t.Fatal("reporter did not stop when its context was cancelled")
	}
	// Stop must remain safe after the reporter has already returned.
	reporter.Stop()
}

func TestStartHelmProgressReporterWithoutConfigIsInert(t *testing.T) {
	reporter := startHelmProgressReporter(context.Background(), nil, "kubeview", "default", func(string) {
		t.Fatal("a reporter without a REST config must not report")
	})
	reporter.Stop()
}

func waitForReportedLine(t *testing.T, mu *sync.Mutex, reported *[]string, want string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		for _, line := range *reported {
			if line == want {
				mu.Unlock()
				return
			}
		}
		mu.Unlock()
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("line %q was never reported", want)
}
