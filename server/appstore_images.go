package server

import (
	"fmt"
	"strings"

	"github.com/beego/beego/logs"

	"github.com/casosorg/casos/object"
	"github.com/casosorg/casos/store"
)

// InstallImageVulnerabilityGate refuses to install a chart whose images are
// already known to carry CRITICAL findings.
//
// It runs before Helm creates anything, so the operator gets a decision they can
// act on: choose another chart version, or clear the finding from the scan
// results to override. Images with no scan yet are allowed and queued — the gate
// reports what is known, and never guesses.
func InstallImageVulnerabilityGate(images []string) error {
	var blocked []string
	for _, image := range images {
		if image == "" {
			continue
		}
		result, err := object.GetTrivyScanResultByImage(image)
		if err != nil {
			logs.Error("trivy cache lookup %s: %v", image, err)
			continue
		}
		if result == nil {
			object.TriggerScan(image)
			continue
		}
		if result.Status == "done" && result.Critical > 0 {
			blocked = append(blocked, fmt.Sprintf("%s (%d CRITICAL)", image, result.Critical))
		}
	}
	if len(blocked) == 0 {
		return nil
	}
	return fmt.Errorf(
		"this app was not installed because its images have known CRITICAL vulnerabilities: %s — install a chart version with updated images, or remove the image from the Trivy scan results to override",
		strings.Join(blocked, ", "),
	)
}

// RegisterInstallImageVulnerabilityGate hands the gate to the store package,
// which cannot reach the scan results itself.
func RegisterInstallImageVulnerabilityGate() {
	store.ImageVulnerabilityGate = InstallImageVulnerabilityGate
}
