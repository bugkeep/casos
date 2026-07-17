package store

import (
	"fmt"

	"helm.sh/helm/v3/pkg/action"
)

func helmActionRecoveryError(actionErr, recoveryErr error) error {
	if recoveryErr == nil {
		return actionErr
	}
	return fmt.Errorf("%w; recovery failed: %v", actionErr, recoveryErr)
}

func uninstallHelmReleaseAfterReadinessFailure(actionConfig *action.Configuration, releaseName string) error {
	uninstall := action.NewUninstall(actionConfig)
	uninstall.IgnoreNotFound = true
	uninstall.Wait = true
	uninstall.Timeout = helmOperationTimeout
	_, err := uninstall.Run(releaseName)
	return err
}

func restoreHelmReleaseRevision(actionConfig *action.Configuration, releaseName string, revision int) error {
	rollback := action.NewRollback(actionConfig)
	rollback.Version = revision
	rollback.Wait = true
	rollback.WaitForJobs = true
	rollback.Timeout = helmOperationTimeout
	return rollback.Run(releaseName)
}

func currentHelmReleaseRevision(actionConfig *action.Configuration, releaseName string) (int, error) {
	release, err := actionConfig.Releases.Last(releaseName)
	if err != nil {
		return 0, fmt.Errorf("read current Helm release %s: %w", releaseName, err)
	}
	return release.Version, nil
}
