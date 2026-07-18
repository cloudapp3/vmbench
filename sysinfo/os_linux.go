//go:build linux

package sysinfo

import (
	"context"
	"os"
	"strings"

	ghost "github.com/shirou/gopsutil/v4/host"
)

func collectOSInfo(ctx context.Context) (OSInfo, []string) {
	name := hostPlatformName(ctx)
	kernel := ""
	if info, err := ghost.InfoWithContext(ctx); err == nil {
		kernel = strings.TrimSpace(info.KernelVersion)
	}
	if kernel == "" {
		if data, err := os.ReadFile("/proc/version"); err == nil {
			kernel = strings.TrimSpace(string(data))
		}
	}
	return buildOSInfo(name, kernel)
}
