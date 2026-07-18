//go:build darwin

package sysinfo

import (
	"context"
	"runtime"
	"strconv"
	"strings"

	gcpu "github.com/shirou/gopsutil/v4/cpu"
)

func collectCPUInfo(ctx context.Context) (CPUInfo, []string) {
	warnings := make([]string, 0, 4)
	logical, _ := gcpu.CountsWithContext(ctx, true)
	physical, _ := gcpu.CountsWithContext(ctx, false)
	brand, err := runCommand(ctx, "sysctl", "-n", "machdep.cpu.brand_string")
	if err != nil {
		brand = ""
	}
	if brand == "" {
		if infos, infoErr := gcpu.InfoWithContext(ctx); infoErr == nil && len(infos) > 0 {
			brand = infos[0].ModelName
		} else if infoErr != nil {
			warnings = append(warnings, "cpu: "+infoErr.Error())
		}
	}
	base := parseSysctlMHz(ctx, "hw.cpufrequency")
	max := parseSysctlMHz(ctx, "hw.cpufrequency_max")
	features := cpuidFeatureList()
	info := CPUInfo{
		Model:         strings.TrimSpace(brand),
		Arch:          runtime.GOARCH,
		PhysicalCores: physical,
		LogicalCores:  logical,
		BaseFreqMHz:   base,
		MaxFreqMHz:    max,
		CacheSizes:    defaultCacheSizes(),
		Features:      features,
		MicroArch:     detectMicroArch(brand, runtime.GOARCH, features),
		NumaNodes:     1,
	}
	return info, warnings
}

func parseSysctlMHz(ctx context.Context, key string) float64 {
	text, err := runCommand(ctx, "sysctl", "-n", key)
	if err != nil {
		return 0
	}
	value, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
	if err != nil || value <= 0 {
		return 0
	}
	return value / 1e6
}
