package store

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"k8s.io/client-go/rest"
)

const (
	// How often an install or upgrade that is still waiting reports what it is
	// waiting for. Helm's own wait only repeats "not ready"; the reason a Pod is
	// not ready — an image still pulling, a node with nowhere to put it — lives
	// in the Pod status and the namespace events, so without this an operator
	// watches twenty minutes of identical lines that never name a cause.
	//
	// Kept under the install dialog's 30s stream idle timeout, so an operation
	// that is merely waiting keeps its stream instead of being demoted to
	// polling.
	helmProgressInterval = 15 * time.Second

	// A log line already sent inside this window is dropped as a repeat.
	helmLogRepeatWindow = time.Minute

	// How many distinct lines the repeat filter remembers before it prunes.
	helmLogRepeatMaxEntries = 512
)

// helmLogThrottle drops a log line that was already sent within the repeat
// window. Helm re-logs the same "Deployment is not ready" line every two
// seconds for as long as its wait lasts, so a twenty-minute install persists
// six hundred identical rows and buries every line that does say something.
// The progress reporter supplies the heartbeat those repeats stood in for.
type helmLogThrottle struct {
	mu     sync.Mutex
	window time.Duration
	sentAt map[string]time.Time
	now    func() time.Time
}

func newHelmLogThrottle() *helmLogThrottle {
	return &helmLogThrottle{
		window: helmLogRepeatWindow,
		sentAt: map[string]time.Time{},
		now:    time.Now,
	}
}

// allow reports whether message should still be sent, recording it when it is.
func (t *helmLogThrottle) allow(message string) bool {
	if t == nil || message == "" {
		return true
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.now()
	if sentAt, ok := t.sentAt[message]; ok && now.Sub(sentAt) < t.window {
		return false
	}
	if len(t.sentAt) >= helmLogRepeatMaxEntries {
		t.prune(now)
	}
	t.sentAt[message] = now
	return true
}

// prune drops entries that can no longer suppress anything. An operation whose
// lines are all distinct and all recent has nothing to prune, and there the
// filter forgets everything rather than growing without bound: repeating a line
// once more costs a row, remembering every line forever costs the process.
func (t *helmLogThrottle) prune(now time.Time) {
	for message, sentAt := range t.sentAt {
		if now.Sub(sentAt) >= t.window {
			delete(t.sentAt, message)
		}
	}
	if len(t.sentAt) >= helmLogRepeatMaxEntries {
		t.sentAt = map[string]time.Time{}
	}
}

// helmProgressReporter explains, while Helm waits, why the release is not ready
// yet. Helm's wait is a single blocking call that reports only that it is still
// waiting, so the diagnostics that would otherwise appear once the operation
// finally fails are collected on an interval and reported as they change.
type helmProgressReporter struct {
	stop     chan struct{}
	done     chan struct{}
	stopOnce sync.Once
}

// startHelmProgressReporter begins reporting the state of a release that an
// operation is waiting on. Stop must be called before the caller stops
// accepting log lines, and reporting ends on its own when ctx is done.
func startHelmProgressReporter(ctx context.Context, cfg *rest.Config, releaseName, namespace string, report func(string)) *helmProgressReporter {
	reporter := &helmProgressReporter{stop: make(chan struct{}), done: make(chan struct{})}
	if cfg == nil || report == nil {
		close(reporter.done)
		return reporter
	}
	if ctx == nil {
		ctx = context.Background()
	}
	go reporter.run(ctx, helmProgressInterval, releaseName, namespace, func() []string {
		return helmReleaseDiagnostics(ctx, cfg, releaseName, namespace)
	}, report)
	return reporter
}

func (r *helmProgressReporter) run(ctx context.Context, interval time.Duration, releaseName, namespace string, diagnose func() []string, report func(string)) {
	defer close(r.done)
	// A timer rather than a ticker: collecting diagnostics from a cluster that
	// is struggling can itself be slow, and the interval is meant to be the gap
	// between reports, not a queue of overdue ones.
	timer := time.NewTimer(interval)
	defer timer.Stop()
	startedAt := time.Now()
	reported := map[string]bool{}
	for {
		select {
		case <-r.stop:
			return
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		report(fmt.Sprintf(
			"still waiting for %s/%s to become ready (%s elapsed)",
			namespace, releaseName, time.Since(startedAt).Round(time.Second),
		))
		// Only what changed since the last round: the first report is the whole
		// picture, and every one after it is news.
		for _, line := range diagnose() {
			if isHelmDiagnosticsHeading(line) || reported[line] {
				continue
			}
			reported[line] = true
			report(line)
		}
		timer.Reset(interval)
	}
}

// Stop ends reporting and waits for the reporter to release the log callback.
func (r *helmProgressReporter) Stop() {
	if r == nil {
		return
	}
	r.stopOnce.Do(func() { close(r.stop) })
	<-r.done
}

// isHelmDiagnosticsHeading matches the two lines that only restate the release
// and its label selector. The progress heading already names the release, and
// dropping the first of them also keeps these reports from being mistaken for
// the post-failure diagnostics that helmUpgradeDiagnosticCollector collects.
func isHelmDiagnosticsHeading(line string) bool {
	return strings.HasPrefix(line, "Helm release diagnostics") || strings.HasPrefix(line, "  selector: ")
}
