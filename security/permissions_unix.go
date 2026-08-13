//go:build !windows

package security

import "syscall"

func HardenProcessFilePermissions() {
	syscall.Umask(0o077)
}
