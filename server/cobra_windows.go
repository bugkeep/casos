//go:build windows

package server

import "github.com/spf13/cobra"

func init() {
	// CasOS embeds Cobra-based Kubernetes components and supports Explorer startup.
	cobra.MousetrapHelpText = ""
}
