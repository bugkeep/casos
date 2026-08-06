package deploy

import (
	"encoding/base64"
	"fmt"
	"strings"
)

func stableUniqueNonEmpty(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
	}
	return result
}

func planContainerdFiles(
	desired []containerdFileSpec,
	current map[string]containerdFileSnapshot,
	adoption containerdConfigAdoptionDecision,
) ([]containerdFileChange, []string, error) {
	var changes []containerdFileChange
	var warnings []string
	var blocked []string
	proxyEnv := current[containerdProxyEnvPath]
	dropIn := current[containerdEgressDropInPath]
	proxyPairUnmanaged := proxyEnv.Kind == containerdFileUnmanaged || dropIn.Kind == containerdFileUnmanaged
	proxyEnabled := false
	for _, file := range desired {
		if isContainerdProxyFile(file.Path) && file.Present {
			proxyEnabled = true
		}
	}
	if proxyPairUnmanaged && proxyEnabled {
		blocked = append(blocked, "containerd proxy cannot be enabled while its env or systemd drop-in path is unmanaged")
	} else if proxyPairUnmanaged && unmanagedContainerdProxyPairActive(proxyEnv, dropIn) {
		blocked = append(blocked, "containerd proxy cannot be disabled while an unmanaged env or systemd drop-in path may still enable it")
	} else if proxyPairUnmanaged {
		warnings = append(warnings, "preserved unmanaged containerd proxy env and systemd drop-in files")
	}
	for _, file := range desired {
		if proxyPairUnmanaged && isContainerdProxyFile(file.Path) {
			// The env file and drop-in form one configuration unit. If either
			// path is operator-owned, never plan a partial update of the pair.
			continue
		}
		snapshot, ok := current[file.Path]
		if !ok {
			snapshot = containerdFileSnapshot{Kind: containerdFileAbsent}
		}
		managed := snapshot.Kind == containerdFileManaged || matchesLegacyContainerdContent(snapshot.Content, file.Legacy)
		if file.Path == containerdConfigPath && snapshot.Kind == containerdFileUnmanaged && adoption.Allowed {
			managed = true
			warnings = append(warnings, fmt.Sprintf("adopting unmanaged containerd config.toml: %s", adoption.Reason))
		}
		if snapshot.Kind == containerdFileUnmanaged && !managed {
			if file.BlockIfUnmanaged {
				if file.Path == containerdConfigPath {
					blocked = append(blocked, "containerd config is unmanaged; migrate or remove it, or retry with adoptContainerdConfig=true to back it up and replace it")
				} else {
					blocked = append(blocked, fmt.Sprintf("containerd file %s is unmanaged; migrate or remove it before applying the selected registry route", file.Path))
				}
				continue
			}
			if file.Path == containerdConfigPath && file.Present {
				blocked = append(blocked, "containerd config.toml is operator-managed; back it up and remove or migrate it before worker deployment")
				continue
			}
			if file.Path == dockerHubHostsPath || file.Path == k8sRegistryHostsPath {
				warnings = append(warnings, fmt.Sprintf("preserved operator-managed containerd registry file %s; its routing takes precedence over imageRegistryMirror for this namespace", file.Path))
			} else {
				warnings = append(warnings, fmt.Sprintf("preserved unmanaged containerd file %s", file.Path))
			}
			continue
		}

		switch {
		case file.Present && snapshot.Kind == containerdFileAbsent:
			changes = append(changes, containerdFileChange{Spec: file, Before: snapshot})
		case file.Present && (snapshot.Content != file.Content || snapshot.Mode != file.Mode):
			changes = append(changes, containerdFileChange{Spec: file, Before: snapshot})
		case !file.Present && managed:
			changes = append(changes, containerdFileChange{Spec: file, Before: snapshot})
		}
	}
	if len(blocked) > 0 {
		return nil, warnings, fmt.Errorf("containerd desired state is blocked: %s", strings.Join(blocked, "; "))
	}
	return changes, warnings, nil
}

func isContainerdProxyFile(path string) bool {
	return path == containerdProxyEnvPath || path == containerdEgressDropInPath
}

func unmanagedContainerdProxyPairActive(env, dropIn containerdFileSnapshot) bool {
	return unmanagedContainerdProxyFileActive(env, true) || unmanagedContainerdProxyFileActive(dropIn, false)
}

func unmanagedContainerdProxyFileActive(snapshot containerdFileSnapshot, envFile bool) bool {
	if snapshot.Kind != containerdFileUnmanaged {
		return false
	}
	if strings.TrimSpace(snapshot.Content) == "" {
		return true
	}
	if envFile {
		for _, line := range strings.Split(snapshot.Content, "\n") {
			parts := strings.SplitN(strings.TrimSpace(line), "=", 2)
			if len(parts) != 2 {
				continue
			}
			key := strings.ToUpper(strings.TrimSpace(parts[0]))
			if key != "HTTP_PROXY" && key != "HTTPS_PROXY" && key != "ALL_PROXY" {
				continue
			}
			value := strings.Trim(strings.TrimSpace(parts[1]), "\"'")
			if value != "" {
				return true
			}
		}
		return false
	}
	// An unmanaged drop-in can define proxy variables or load another env file.
	// Treat it as active rather than claiming the worker proxy is disabled.
	return true
}

func matchesLegacyContainerdContent(content string, legacy []string) bool {
	for _, candidate := range legacy {
		if content == candidate {
			return true
		}
	}
	return false
}

func parseContainerdFileSnapshot(output string) (containerdFileSnapshot, error) {
	output = strings.TrimSpace(output)
	switch output {
	case "absent":
		return containerdFileSnapshot{Kind: containerdFileAbsent}, nil
	case "unmanaged":
		return containerdFileSnapshot{Kind: containerdFileUnmanaged}, nil
	}
	parts := strings.SplitN(output, ":", 3)
	if len(parts) != 3 || parts[0] != "regular" {
		return containerdFileSnapshot{}, fmt.Errorf("unexpected remote file state format")
	}
	content, err := base64.StdEncoding.DecodeString(parts[2])
	if err != nil {
		return containerdFileSnapshot{}, fmt.Errorf("decode remote file content: %w", err)
	}
	mode := parts[1]
	if len(mode) == 3 {
		mode = "0" + mode
	}
	kind := containerdFileUnmanaged
	text := string(content)
	if text == generatedContainerdConfigMarker || strings.HasPrefix(text, generatedContainerdConfigMarker+"\n") {
		kind = containerdFileManaged
	}
	return containerdFileSnapshot{Kind: kind, Content: text, Mode: mode}, nil
}
