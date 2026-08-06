package deploy

import (
	"fmt"
	"sort"
	"strings"
)

type containerdVerificationResult string

const (
	containerdVerifiedPulls containerdVerificationResult = "cri-pull:sandbox,verification"
	containerdVerifiedReady containerdVerificationResult = "cri-ready-only"
)

func containerdConfigValidationCommand(verifySystemd bool) string {
	commands := []string{
		"set -e",
		"containerd --config /etc/containerd/config.toml config dump >/dev/null",
	}
	if verifySystemd {
		commands = append(commands, "systemd-analyze verify containerd.service >/dev/null")
	}
	return strings.Join(commands, "\n")
}

func containerdCRIVerificationCommand(sandboxImage, verificationImage string) string {
	sandboxImage = strings.TrimSpace(sandboxImage)
	verificationImage = strings.TrimSpace(verificationImage)
	if verificationImage == "" {
		verificationImage = defaultContainerdVerificationImage
	}
	return fmt.Sprintf(`set -e
# verify-containerd-cri
systemctl is-active --quiet containerd
if command -v crictl >/dev/null 2>&1; then
  if [ -n %s ]; then
    crictl --runtime-endpoint unix:///run/containerd/containerd.sock pull %s >/dev/null
  fi
  if [ -n %s ]; then
    crictl --runtime-endpoint unix:///run/containerd/containerd.sock pull %s >/dev/null
  fi
  printf '%%s' 'cri-pull:sandbox,verification'
else
  ctr plugins ls | awk '$1 == "io.containerd.grpc.v1" && $2 == "cri" && $NF == "ok" { found=1 } END { exit !found }'
  printf '%%s' cri-ready-only
fi`, shellSingleQuote(sandboxImage), shellSingleQuote(sandboxImage), shellSingleQuote(verificationImage), shellSingleQuote(verificationImage))
}

func sortedContainerdChangedPaths(result containerdReconcileResult) []string {
	paths := append([]string(nil), result.ChangedPaths...)
	sort.Strings(paths)
	return paths
}
