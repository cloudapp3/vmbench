package sysinfo

import (
	"context"

	ghost "github.com/shirou/gopsutil/v4/host"
	gload "github.com/shirou/gopsutil/v4/load"
	gmem "github.com/shirou/gopsutil/v4/mem"
)

// collectHostBasics gathers cross-platform uptime, load, timezone, and swap
// from gopsutil. Failures leave the fields zero without warnings because the
// diagnostics are best-effort evidence.
func collectHostBasics(ctx context.Context) PlatformDiagnostics {
	diagnostics := PlatformDiagnostics{}
	if info, err := ghost.InfoWithContext(ctx); err == nil {
		diagnostics.UptimeSeconds = info.Uptime
	}
	if zone, err := currentTimezone(ctx); err == nil {
		diagnostics.Timezone = zone
	}
	if avg, err := gload.AvgWithContext(ctx); err == nil {
		diagnostics.Load1, diagnostics.Load5, diagnostics.Load15 = avg.Load1, avg.Load5, avg.Load15
	}
	if swap, err := gmem.SwapMemoryWithContext(ctx); err == nil {
		diagnostics.SwapTotalBytes = swap.Total
		diagnostics.SwapUsedBytes = swap.Used
	}
	diagnostics.normalize()
	return diagnostics
}
