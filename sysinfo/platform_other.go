//go:build !linux

package sysinfo

import (
	"context"
	"time"
)

// currentTimezone returns the system timezone name on non-Linux platforms.
func currentTimezone(ctx context.Context) (string, error) {
	name, _ := time.Now().Zone()
	return name, nil
}

// collectLinuxKernelTuning is Linux-only evidence; other platforms return an
// empty report so the diagnostics stay additive.
func collectLinuxKernelTuning() PlatformDiagnostics { return PlatformDiagnostics{} }
