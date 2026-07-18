//go:build windows

package sysinfo

import (
	"context"
	"runtime"
	"strings"

	gcpu "github.com/shirou/gopsutil/v4/cpu"
)

func collectCPUInfo(ctx context.Context) (CPUInfo, []string) {
	warnings := make([]string, 0, 2)
	logical, _ := gcpu.CountsWithContext(ctx, true)
	physical, _ := gcpu.CountsWithContext(ctx, false)
	infos, err := gcpu.InfoWithContext(ctx)
	if err != nil {
		warnings = append(warnings, "cpu: "+err.Error())
	}
	info := CPUInfo{
		Arch:          runtime.GOARCH,
		LogicalCores:  logical,
		PhysicalCores: physical,
		CacheSizes:    defaultCacheSizes(),
		Features:      cpuidFeatureList(),
		NumaNodes:     1,
	}
	if len(infos) > 0 {
		info.Model = strings.TrimSpace(infos[0].ModelName)
		info.BaseFreqMHz = infos[0].Mhz
		info.MaxFreqMHz = infos[0].Mhz
	}
	info.MicroArch = detectMicroArch(info.Model, info.Arch, info.Features)
	return info, warnings
}
