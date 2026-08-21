package store

import (
	"context"
	"net/url"
	"sync"
	"time"
)

// HelmChartLoadStage names the step a chart load is currently on.
type HelmChartLoadStage string

const (
	HelmChartLoadStageIndex  HelmChartLoadStage = "index"
	HelmChartLoadStageChart  HelmChartLoadStage = "chart"
	HelmChartLoadStageOCI    HelmChartLoadStage = "oci"
	HelmChartLoadStageRender HelmChartLoadStage = "render"
)

// helmChartLoadProgressInterval throttles byte-count updates. A 27 MB index
// read in 32 KB chunks would otherwise report about a thousand times.
const helmChartLoadProgressInterval = 250 * time.Millisecond

// HelmChartLoadProgress is one progress update from a chart load. Total is 0
// when the server sends no Content-Length, which consumers must treat as "size
// unknown" rather than inventing a percentage.
type HelmChartLoadProgress struct {
	Stage  HelmChartLoadStage `json:"stage"`
	Loaded int64              `json:"loaded"`
	Total  int64              `json:"total"`
	Host   string             `json:"host,omitempty"`
}

type helmChartLoadProgressFunc func(HelmChartLoadProgress)

type helmChartLoadReporterKey struct{}

type helmChartLoadStageKey struct{}

// withHelmChartLoadProgress returns a context whose chart downloads report
// progress to report. A nil report leaves the context unchanged.
func withHelmChartLoadProgress(ctx context.Context, report helmChartLoadProgressFunc) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if report == nil {
		return ctx
	}
	return context.WithValue(ctx, helmChartLoadReporterKey{}, report)
}

// withHelmChartLoadStage labels the downloads made through the returned context.
func withHelmChartLoadStage(ctx context.Context, stage HelmChartLoadStage) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, helmChartLoadStageKey{}, stage)
}

func helmChartLoadReporter(ctx context.Context) helmChartLoadProgressFunc {
	if ctx == nil {
		return nil
	}
	report, _ := ctx.Value(helmChartLoadReporterKey{}).(helmChartLoadProgressFunc)
	return report
}

func helmChartLoadStage(ctx context.Context) HelmChartLoadStage {
	if ctx == nil {
		return ""
	}
	stage, _ := ctx.Value(helmChartLoadStageKey{}).(HelmChartLoadStage)
	return stage
}

// reportHelmChartLoadStage announces a stage that moves no bytes of its own.
func reportHelmChartLoadStage(ctx context.Context, stage HelmChartLoadStage) {
	report := helmChartLoadReporter(ctx)
	if report == nil {
		return
	}
	report(HelmChartLoadProgress{Stage: stage})
}

// helmChartLoadProgressReader counts bytes on their way out of an HTTP body and
// reports them at a fixed interval. It counts what crosses the network, so a
// gzipped body is measured compressed — the same units Content-Length carries.
type helmChartLoadProgressReader struct {
	stage    HelmChartLoadStage
	host     string
	total    int64
	loaded   int64
	report   helmChartLoadProgressFunc
	interval time.Duration
	lastSent time.Time
	now      func() time.Time
}

func newHelmChartLoadProgressReader(ctx context.Context, rawURL string, total int64) *helmChartLoadProgressReader {
	report := helmChartLoadReporter(ctx)
	if report == nil {
		return nil
	}
	if total < 0 {
		total = 0
	}
	reader := &helmChartLoadProgressReader{
		stage:    helmChartLoadStage(ctx),
		host:     helmChartLoadProgressHost(rawURL),
		total:    total,
		report:   report,
		interval: helmChartLoadProgressInterval,
		now:      time.Now,
	}
	// An immediate zero-byte update changes the stage label when the request is
	// made, not after the first chunk — which on a stalled connection may never
	// arrive.
	reader.emit()
	return reader
}

func (r *helmChartLoadProgressReader) Write(p []byte) (int, error) {
	if r == nil {
		return len(p), nil
	}
	r.loaded += int64(len(p))
	if r.now().Sub(r.lastSent) >= r.interval {
		r.emit()
	}
	return len(p), nil
}

// Finish emits the byte count one last time, so a completed download does not
// sit at whatever the last throttled update happened to be.
func (r *helmChartLoadProgressReader) Finish() {
	if r == nil {
		return
	}
	r.emit()
}

func (r *helmChartLoadProgressReader) emit() {
	r.lastSent = r.now()
	r.report(HelmChartLoadProgress{
		Stage:  r.stage,
		Loaded: r.loaded,
		Total:  r.total,
		Host:   r.host,
	})
}

func helmChartLoadProgressHost(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return parsed.Host
}

// helmChartLoadProgressCollector adapts progress callbacks to a channel an SSE
// handler can range over. Progress is dropped rather than queued when the
// consumer falls behind: blocking here would slow the transfer being reported.
type helmChartLoadProgressCollector struct {
	events chan HelmChartValuesStreamEvent
	mu     sync.Mutex
	closed bool
}

func newHelmChartLoadProgressCollector(buffer int) *helmChartLoadProgressCollector {
	return &helmChartLoadProgressCollector{events: make(chan HelmChartValuesStreamEvent, buffer)}
}

func (c *helmChartLoadProgressCollector) report(progress HelmChartLoadProgress) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	select {
	case c.events <- HelmChartValuesStreamEvent{Type: HelmChartValuesStreamEventProgress, Progress: &progress}:
	default:
	}
}

// send delivers a terminal event, waiting for room because losing it would
// leave the client without a result.
func (c *helmChartLoadProgressCollector) send(ctx context.Context, event HelmChartValuesStreamEvent) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	select {
	case c.events <- event:
	case <-ctx.Done():
	}
}

func (c *helmChartLoadProgressCollector) close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	c.closed = true
	close(c.events)
}
