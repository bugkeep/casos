//go:build !embed

package conf

// A build that is not standalone always reads conf/app.conf from disk, so there
// is nothing compiled in to fall back to.
func loadEmbeddedConfig() {}

// IsEmbeddedConfig reports whether CasOS is running on a compiled-in
// conf/app.conf, which only a standalone build can do.
func IsEmbeddedConfig() bool { return false }
