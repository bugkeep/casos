package store

import (
	"fmt"
	"strings"
	"time"

	"helm.sh/helm/v3/pkg/action"

	"github.com/casosorg/casos/conf"
)

const defaultHelmInstallTimeout = 20 * time.Minute

func configuredHelmInstallTimeout() (time.Duration, error) {
	return parseHelmInstallTimeout(conf.GetConfigString("helmInstallTimeout"))
}

func parseHelmInstallTimeout(raw string) (time.Duration, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return defaultHelmInstallTimeout, nil
	}
	timeout, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid helmInstallTimeout %q: %w", raw, err)
	}
	if timeout <= 0 {
		return 0, fmt.Errorf("helmInstallTimeout must be greater than zero")
	}
	return timeout, nil
}

func configureHelmInstall(install *action.Install, releaseName, namespace string, timeout time.Duration) {
	install.ReleaseName = releaseName
	install.Namespace = namespace
	install.CreateNamespace = true
	install.Wait = true
	install.WaitForJobs = true
	install.Atomic = true
	install.Timeout = timeout
	install.PostRenderer = configuredLocalImagePullPolicyPostRenderer()
}
