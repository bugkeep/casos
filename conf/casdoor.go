package conf

import "sync/atomic"

var casdoorAvailable atomic.Bool

// SetCasdoorAvailable records whether Casdoor was initialized successfully.
func SetCasdoorAvailable(available bool) {
	casdoorAvailable.Store(available)
}

// IsCasdoorAvailable reports whether Casdoor is configured and usable for SDK calls.
func IsCasdoorAvailable() bool {
	return casdoorAvailable.Load()
}
