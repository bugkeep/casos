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

func planContainerdFiles(desired []containerdDesiredFile, current map[string]containerdFileSnapshot) containerdFilePlan {
	plan := containerdFilePlan{}
	proxyEnv := current[containerdProxyEnvPath]
	dropIn := current[containerdEgressDropInPath]
	proxyPairUnmanaged := proxyEnv.Kind == containerdFileUnmanaged || dropIn.Kind == containerdFileUnmanaged
	proxyEnabled := false
	for _, file := range desired {
		if (file.Role == containerdFileRoleProxyEnv || file.Role == containerdFileRoleProxyDropIn) && file.State == containerdDesiredPresent {
			proxyEnabled = true
		}
	}
	if proxyPairUnmanaged && proxyEnabled {
		plan.Blocked = append(plan.Blocked, "containerd proxy cannot be enabled while its env or systemd drop-in path is unmanaged")
	} else if proxyPairUnmanaged && unmanagedContainerdProxyPairActive(proxyEnv, dropIn) {
		plan.Blocked = append(plan.Blocked, "containerd proxy cannot be disabled while an unmanaged env or systemd drop-in path may still enable it")
	}
	for _, file := range desired {
		snapshot, ok := current[file.Path]
		if !ok {
			snapshot = containerdFileSnapshot{Kind: containerdFileAbsent}
		}
		managed := snapshot.Kind == containerdFileManaged || matchesLegacyContainerdContent(snapshot.Content, file.Legacy)
		if snapshot.Kind == containerdFileUnmanaged && !managed {
			if file.BlockIfUnmanaged && file.State == containerdDesiredPresent {
				plan.Blocked = append(plan.Blocked, fmt.Sprintf("required containerd file %s is unmanaged; migrate or explicitly adopt it before applying the selected registry route", file.Path))
				continue
			}
			if file.Role == containerdFileRoleConfig && file.State == containerdDesiredPresent {
				plan.Blocked = append(plan.Blocked, "containerd config.toml is unmanaged; migrate or explicitly adopt it before CasOS can manage egress")
				continue
			}
			plan.Warnings = append(plan.Warnings, fmt.Sprintf("preserved unmanaged containerd file %s", file.Path))
			continue
		}

		var change *containerdFileChange
		switch {
		case file.State == containerdDesiredPresent && snapshot.Kind == containerdFileAbsent:
			change = &containerdFileChange{Path: file.Path, Action: containerdFileWrite, Content: file.Content, Mode: file.Mode, Previous: snapshot}
		case file.State == containerdDesiredPresent && (snapshot.Content != file.Content || snapshot.Mode != file.Mode):
			change = &containerdFileChange{Path: file.Path, Action: containerdFileWrite, Content: file.Content, Mode: file.Mode, Previous: snapshot}
		case file.State == containerdDesiredAbsent && managed:
			change = &containerdFileChange{Path: file.Path, Action: containerdFileRemove, Previous: snapshot}
		}
		if change == nil {
			continue
		}
		plan.Changes = append(plan.Changes, *change)
		plan.DaemonReload = plan.DaemonReload || file.DaemonReload
		plan.Restart = plan.Restart || file.Restart
	}
	return plan
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
		return containerdFileSnapshot{}, fmt.Errorf("unexpected remote file state %q", output)
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
