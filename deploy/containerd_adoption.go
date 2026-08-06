package deploy

import (
	"context"
	"fmt"
	"strings"
)

type containerdPackageConfigState string

const (
	containerdPackageConfigDefault    containerdPackageConfigState = "package-default"
	containerdPackageConfigModified   containerdPackageConfigState = "modified"
	containerdPackageConfigUnverified containerdPackageConfigState = "unverified"
)

type containerdConfigAdoptionDecision struct {
	Allowed        bool
	BackupRequired bool
	Reason         string
}

type containerdConfigAdoptionRunner interface {
	InspectFileContext(ctx context.Context, path string) (containerdFileSnapshot, error)
	RunRootContext(ctx context.Context, command string) (string, error)
}

func decideContainerdConfigAdoption(
	installedBefore bool,
	before containerdFileSnapshot,
	after containerdFileSnapshot,
	packageState containerdPackageConfigState,
) containerdConfigAdoptionDecision {
	if after.Kind != containerdFileUnmanaged || after.Mode == "" {
		return containerdConfigAdoptionDecision{}
	}
	if !installedBefore && before.Kind == containerdFileAbsent {
		return containerdConfigAdoptionDecision{Allowed: true, Reason: "config created by the current package installation"}
	}
	if packageState == containerdPackageConfigDefault {
		return containerdConfigAdoptionDecision{Allowed: true, Reason: "unmodified package-default config"}
	}
	return containerdConfigAdoptionDecision{}
}

func inspectContainerdPackageInstalled(ctx context.Context, runner interface {
	RunRootContext(context.Context, string) (string, error)
},
) (bool, error) {
	output, err := runner.RunRootContext(ctx, `# inspect-containerd-package-installed
if dpkg-query -W -f='${db:Status-Status}' containerd 2>/dev/null | grep -qx installed; then
  printf installed
else
  printf absent
fi`)
	if err != nil {
		return false, fmt.Errorf("inspect containerd package state: %w", err)
	}
	return parseContainerdPackageInstalled(output)
}

func parseContainerdPackageInstalled(output string) (bool, error) {
	switch strings.TrimSpace(output) {
	case "installed":
		return true, nil
	case "absent":
		return false, nil
	default:
		return false, fmt.Errorf("unexpected containerd package state %q", strings.TrimSpace(output))
	}
}

func resolveContainerdConfigAdoption(
	ctx context.Context,
	runner containerdConfigAdoptionRunner,
	installedBefore bool,
	before containerdFileSnapshot,
	adoptExisting bool,
) (containerdConfigAdoptionDecision, error) {
	after, err := runner.InspectFileContext(ctx, containerdConfigPath)
	if err != nil {
		return containerdConfigAdoptionDecision{}, fmt.Errorf("inspect containerd config after package installation: %w", err)
	}
	decision := decideContainerdConfigAdoption(installedBefore, before, after, containerdPackageConfigUnverified)
	if decision.Allowed || after.Kind != containerdFileUnmanaged || after.Mode == "" {
		return decision, nil
	}
	packageState, err := inspectContainerdPackageConfigState(ctx, runner)
	if err != nil {
		return containerdConfigAdoptionDecision{}, err
	}
	decision = decideContainerdConfigAdoption(installedBefore, before, after, packageState)
	if decision.Allowed || !adoptExisting {
		return decision, nil
	}
	return containerdConfigAdoptionDecision{
		Allowed:        true,
		BackupRequired: true,
		Reason:         "explicitly adopted by the deployment request after preserving the original file",
	}, nil
}

func backupContainerdConfigForAdoption(
	ctx context.Context,
	runner interface {
		RunRootContext(context.Context, string) (string, error)
	},
	decision containerdConfigAdoptionDecision,
) (string, error) {
	if !decision.BackupRequired {
		return "", nil
	}
	output, err := runner.RunRootContext(ctx, `set -e
path=/etc/containerd/config.toml
test -f "$path"
test ! -L "$path"
timestamp=$(date -u +%Y%m%dT%H%M%SZ)
backup="${path}.casos-backup-${timestamp}"
suffix=0
while [ -e "$backup" ]; do
  suffix=$((suffix + 1))
  backup="${path}.casos-backup-${timestamp}-${suffix}"
done
cp --preserve=all -- "$path" "$backup"
printf '%s' "$backup"`)
	if err != nil {
		return "", fmt.Errorf("back up existing containerd config: %w", err)
	}
	backupPath := strings.TrimSpace(output)
	if !strings.HasPrefix(backupPath, containerdConfigPath+".casos-backup-") || strings.ContainsAny(backupPath, "\r\n") {
		return "", fmt.Errorf("back up existing containerd config: unexpected backup path %q", backupPath)
	}
	return backupPath, nil
}

func inspectContainerdPackageConfigState(ctx context.Context, runner interface {
	RunRootContext(context.Context, string) (string, error)
},
) (containerdPackageConfigState, error) {
	output, err := runner.RunRootContext(ctx, `set -e
# inspect-containerd-package-config
path=/etc/containerd/config.toml
if [ ! -f "$path" ]; then
  printf unverified
  exit 0
fi
expected=$(dpkg-query -W -f='${Conffiles}\n' containerd 2>/dev/null | awk '$1 == "/etc/containerd/config.toml" { print $2; exit }')
if [ -z "$expected" ] && [ -f /var/lib/dpkg/info/containerd.md5sums ]; then
  expected=$(awk '$2 == "etc/containerd/config.toml" { print $1; exit }' /var/lib/dpkg/info/containerd.md5sums)
fi
if [ -z "$expected" ]; then
  printf unverified
  exit 0
fi
actual=$(md5sum "$path" | awk '{ print $1 }')
if [ "$actual" = "$expected" ]; then
  printf package-default
else
  printf modified
fi`)
	if err != nil {
		return containerdPackageConfigUnverified, fmt.Errorf("inspect package ownership of containerd config: %w", err)
	}
	switch state := containerdPackageConfigState(strings.TrimSpace(output)); state {
	case containerdPackageConfigDefault, containerdPackageConfigModified, containerdPackageConfigUnverified:
		return state, nil
	default:
		return containerdPackageConfigUnverified, fmt.Errorf("unexpected containerd package config state %q", state)
	}
}
