package store

import (
	"context"
	"fmt"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func validateHelmChartCompatibility(ctx context.Context, cfg *rest.Config, actionConfig *action.Configuration, releaseName, namespace string, chartToInstall *chart.Chart, values map[string]interface{}) error {
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
	namespaceExists, err := helmNamespaceExists(ctx, cfg, namespace)
	if err != nil {
		return err
	}
	// Helm's dry-run validates rendering and resource structure against the
	// target cluster. Remote template lookups are only safe after the release
	// namespace exists; the real install remains the source of truth.
	dryRun := newHelmCompatibilityDryRun(actionConfig, releaseName, namespace, namespaceExists)
	_, err = dryRun.RunWithContext(ctx, chartToInstall, values)
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

func validateHelmRollbackCompatibility(ctx context.Context, actionConfig *action.Configuration, releaseName, namespace string, revision int) error {
	currentRelease, err := actionConfig.Releases.Last(releaseName)
	if err != nil {
		return fmt.Errorf("read current Helm release %s: %w", releaseName, err)
	}
	targetRevision := revision
	if targetRevision == 0 {
		targetRevision = currentRelease.Version - 1
	}
	if targetRevision <= 0 {
		return fmt.Errorf("release %s has no prior revision to roll back to", releaseName)
	}
	target, err := actionConfig.Releases.Get(releaseName, targetRevision)
	if err != nil {
		return fmt.Errorf("read Helm release %s revision %d: %w", releaseName, targetRevision, err)
	}
	if err := validateHelmReleaseCompatibility(ctx, actionConfig, releaseName, namespace, target.Chart, target.Config); err != nil {
		return err
	}
	return nil
}

func newHelmCompatibilityDryRun(actionConfig *action.Configuration, releaseName, namespace string, remoteLookup bool) *action.Install {
	dryRun := action.NewInstall(actionConfig)
	dryRun.ReleaseName = releaseName
	dryRun.Namespace = namespace
	dryRun.CreateNamespace = true
	dryRun.DryRun = true
	dryRun.DryRunOption = "client"
	if remoteLookup {
		dryRun.DryRunOption = "server"
	}
	dryRun.Timeout = helmCompatibilityTimeout
	return dryRun
}

func helmNamespaceExists(ctx context.Context, cfg *rest.Config, namespace string) (bool, error) {
	if cfg == nil {
		return false, fmt.Errorf("Helm compatibility REST config is nil")
	}
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return false, fmt.Errorf("create Helm compatibility client: %w", err)
	}
	if _, err := client.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{}); apierrors.IsNotFound(err) {
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("check Helm release namespace %s: %w", namespace, err)
	}
	return true, nil
}
