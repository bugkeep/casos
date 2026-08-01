package server

import (
	"fmt"
	"strings"

	"github.com/casosorg/casos/conf"
)

type RegistryMirrorMode string

// Registry mirror modes accepted in app.conf. Auto is evaluated on each
// target worker so the decision matches the network used by containerd.
const (
	RegistryMirrorModeAuto   RegistryMirrorMode = "auto"
	RegistryMirrorModeAlways RegistryMirrorMode = "always"
	RegistryMirrorModeNever  RegistryMirrorMode = "never"
)

func resolveRegistryMirrorMode() (RegistryMirrorMode, error) {
	mode := RegistryMirrorMode(strings.ToLower(strings.TrimSpace(conf.GetConfigStringDefault("imageRegistryMirror", string(RegistryMirrorModeAuto)))))
	switch mode {
	case RegistryMirrorModeAuto, RegistryMirrorModeAlways, RegistryMirrorModeNever:
		return mode, nil
	default:
		return "", fmt.Errorf("invalid imageRegistryMirror %q in app.conf: expected auto, always, or never", mode)
	}
}
