package store

import (
	"context"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
)

// ImageVulnerabilityGate decides whether a release's images may be installed.
// It is supplied by the caller, because the scan results live in the database
// layer and store stays independent of it. A nil gate installs anything.
//
// This is where the vulnerability policy belongs: at install time the operator
// is making a choice and can answer it — pick another chart version, or accept
// the finding and clear it from the scan results. The admission webhook cannot,
// because by the time it sees a pod the release is already installed and the
// only thing refusing it achieves is destroying a running app.
var ImageVulnerabilityGate func(images []string) error

// checkHelmInstallImages renders the chart client-side and runs the gate over
// the images the release would run. A render that fails is not treated as a
// gate failure: the real install renders again and reports the problem with far
// better context than this check could.
func checkHelmInstallImages(ctx context.Context, actionConfig *action.Configuration, releaseName, namespace string, chartToInstall *chart.Chart, values map[string]interface{}) error {
	gate := ImageVulnerabilityGate
	if gate == nil || chartToInstall == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	// Client-side only: this runs after the compatibility dry-run has already
	// checked the chart against the cluster, so it needs the manifest, not
	// another round of server-side validation.
	dryRun := newHelmCompatibilityDryRun(actionConfig, releaseName, namespace, false)
	rendered, err := dryRun.RunWithContext(ctx, chartToInstall, values)
	if err != nil || rendered == nil {
		return nil
	}
	images := uniqueManifestImages(manifestImages([]byte(rendered.Manifest)))
	if len(images) == 0 {
		return nil
	}
	return gate(images)
}

// uniqueManifestImages keeps the first occurrence of each image. One image
// commonly appears several times in a chart — an init container and the app
// container often share it — and the gate reports what it blocks, so a repeated
// image would be named repeatedly in the same sentence.
func uniqueManifestImages(images []string) []string {
	seen := make(map[string]bool, len(images))
	unique := make([]string, 0, len(images))
	for _, image := range images {
		if image == "" || seen[image] {
			continue
		}
		seen[image] = true
		unique = append(unique, image)
	}
	return unique
}
