//go:build windows

package sysinfo

import (
	"context"
	"strings"

	ghost "github.com/shirou/gopsutil/v4/host"
)

func collectOSInfo(ctx context.Context) (OSInfo, []string) {
	name := hostPlatformName(ctx)
	kernel := ""
	if info, err := ghost.InfoWithContext(ctx); err == nil {
		kernel = strings.TrimSpace(info.KernelVersion)
	}
	return buildOSInfo(name, kernel)
}
