package store

import (
	"context"
	"fmt"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
)

func validateHelmChartCompatibility(ctx context.Context, actionConfig *action.Configuration, releaseName, namespace string, chartToInstall *chart.Chart, values map[string]interface{}) error {
	if chartToInstall == nil || chartToInstall.Metadata == nil {
		return fmt.Errorf("chart metadata is missing")
	}
	if !isInstallableHelmChartMetadata(chartToInstall.Metadata) {
		return fmt.Errorf("chart %s is a library chart and cannot be installed as an application", chartToInstall.Name())
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, helmCompatibilityTimeout)
	defer cancel()
	// Server-side dry-run is the source of truth for compatibility. It uses the
	// target cluster's discovery and admission stack instead of maintaining a
	// second, inevitably stale allowlist of Kubernetes resource kinds.
	dryRun := newHelmCompatibilityDryRun(actionConfig, releaseName, namespace)
	_, err := dryRun.RunWithContext(ctx, chartToInstall, values)
	if err != nil {
		return fmt.Errorf("render chart %s for compatibility check: %w", chartToInstall.Name(), err)
	}
	return nil
}

func validateHelmReleaseCompatibility(ctx context.Context, actionConfig *action.Configuration, releaseName, namespace string, chartToInstall *chart.Chart, values map[string]interface{}) error {
	if chartToInstall == nil || chartToInstall.Metadata == nil {
		return fmt.Errorf("chart metadata is missing")
	}
	if !isInstallableHelmChartMetadata(chartToInstall.Metadata) {
		return fmt.Errorf("chart %s is a library chart and cannot be installed as an application", chartToInstall.Name())
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, helmCompatibilityTimeout)
	defer cancel()

	dryRun := action.NewUpgrade(actionConfig)
	dryRun.Namespace = namespace
	dryRun.DryRun = true
	dryRun.DryRunOption = "server"
	dryRun.Wait = true
	dryRun.WaitForJobs = true
	dryRun.Timeout = helmCompatibilityTimeout
	if _, err := dryRun.RunWithContext(ctx, releaseName, chartToInstall, values); err != nil {
		return fmt.Errorf("render Helm release %s for compatibility check: %w", releaseName, err)
	}
	return nil
}

func validateHelmRollbackCompatibility(ctx context.Context, actionConfig *action.Configuration, releaseName, namespace string, revision int) (int, error) {
	currentRevision, err := currentHelmReleaseRevision(actionConfig, releaseName)
	if err != nil {
		return 0, err
	}
	targetRevision := revision
	if targetRevision == 0 {
		targetRevision = currentRevision - 1
	}
	if targetRevision <= 0 {
		return 0, fmt.Errorf("release %s has no prior revision to roll back to", releaseName)
	}
	target, err := actionConfig.Releases.Get(releaseName, targetRevision)
	if err != nil {
		return 0, fmt.Errorf("read Helm release %s revision %d: %w", releaseName, targetRevision, err)
	}
	if err := validateHelmReleaseCompatibility(ctx, actionConfig, releaseName, namespace, target.Chart, target.Config); err != nil {
		return 0, err
	}
	return currentRevision, nil
}

func newHelmCompatibilityDryRun(actionConfig *action.Configuration, releaseName, namespace string) *action.Install {
	dryRun := action.NewInstall(actionConfig)
	dryRun.ReleaseName = releaseName
	dryRun.Namespace = namespace
	dryRun.CreateNamespace = true
	dryRun.DryRun = true
	dryRun.DryRunOption = "server"
	dryRun.Timeout = helmCompatibilityTimeout
	return dryRun
}
