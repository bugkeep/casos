package store

import "context"

// helmChartValuesStreamBuffer is small because progress updates are dropped
// rather than queued when the client cannot keep up.
const helmChartValuesStreamBuffer = 8

// HelmChartValuesStreamEventType is the kind of event on the values stream.
type HelmChartValuesStreamEventType string

const (
	HelmChartValuesStreamEventProgress HelmChartValuesStreamEventType = "progress"
	HelmChartValuesStreamEventDone     HelmChartValuesStreamEventType = "done"
	HelmChartValuesStreamEventError    HelmChartValuesStreamEventType = "error"
)

// HelmChartValuesStreamEvent is one message on the values stream. Exactly one
// of Progress, Values or Message is set, per Type.
type HelmChartValuesStreamEvent struct {
	Type     HelmChartValuesStreamEventType `json:"type"`
	Progress *HelmChartLoadProgress         `json:"progress,omitempty"`
	Values   string                         `json:"values,omitempty"`
	Message  string                         `json:"message,omitempty"`
}

// GetHelmChartInstallValuesStream loads a chart's install values and reports
// what it is doing while it does it. The channel emits progress updates, then
// exactly one done or error event, and is closed afterwards. Cancelling ctx
// aborts the download.
func GetHelmChartInstallValuesStream(ctx context.Context, chartName, repoURL, version string) <-chan HelmChartValuesStreamEvent {
	return GetHelmChartInstallValuesStreamWithFallback(ctx, chartName, repoURL, version, "")
}

func GetHelmChartInstallValuesStreamWithFallback(ctx context.Context, chartName, repoURL, version, contentURL string) <-chan HelmChartValuesStreamEvent {
	if ctx == nil {
		ctx = context.Background()
	}
	collector := newHelmChartLoadProgressCollector(helmChartValuesStreamBuffer)
	go func() {
		defer collector.close()
		values, err := GetHelmChartInstallValuesWithFallbackContext(
			withHelmChartLoadProgress(ctx, collector.report),
			chartName,
			repoURL,
			version,
			contentURL,
		)
		if err != nil {
			collector.send(ctx, HelmChartValuesStreamEvent{
				Type:    HelmChartValuesStreamEventError,
				Message: err.Error(),
			})
			return
		}
		collector.send(ctx, HelmChartValuesStreamEvent{
			Type:   HelmChartValuesStreamEventDone,
			Values: values,
		})
	}()
	return collector.events
}
