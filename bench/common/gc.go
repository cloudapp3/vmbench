package common

import "runtime/debug"

// WithGCDisabled runs fn while GC is disabled and restores the prior setting afterwards.
func WithGCDisabled(fn func()) {
	previous := debug.SetGCPercent(-1)
	defer debug.SetGCPercent(previous)
	fn()
}
