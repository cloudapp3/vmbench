//go:build linux

package sysinfo

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	gcpu "github.com/shirou/gopsutil/v4/cpu"
)

func collectCPUInfo(ctx context.Context) (CPUInfo, []string) {
	warnings := make([]string, 0, 4)
	logical, _ := gcpu.CountsWithContext(ctx, true)
	physical, _ := gcpu.CountsWithContext(ctx, false)
	infoList, err := gcpu.InfoWithContext(ctx)
	if err != nil {
		warnings = append(warnings, "cpu: "+err.Error())
	}

	info := CPUInfo{
		Arch:          runtime.GOARCH,
		LogicalCores:  logical,
		PhysicalCores: physical,
		CacheSizes:    readLinuxCaches(),
		Features:      cpuidFeatureList(),
		NumaNodes:     countLinuxNUMANodes(),
	}
	if info.NumaNodes == 0 {
		info.NumaNodes = 1
	}
	if len(infoList) > 0 {
		info.Model = strings.TrimSpace(infoList[0].ModelName)
		info.BaseFreqMHz = infoList[0].Mhz
		info.MaxFreqMHz = infoList[0].Mhz
	}
	if info.BaseFreqMHz == 0 {
		info.BaseFreqMHz = readLinuxCPUFreqMHz("/sys/devices/system/cpu/cpu0/cpufreq/base_frequency")
	}
	if info.MaxFreqMHz == 0 {
		info.MaxFreqMHz = readLinuxCPUFreqMHz("/sys/devices/system/cpu/cpu0/cpufreq/cpuinfo_max_freq")
	}
	if info.BaseFreqMHz == 0 {
		info.BaseFreqMHz = readLinuxCPUFreqMHz("/sys/devices/system/cpu/cpu0/cpufreq/scaling_max_freq")
	}
	info.MicroArch = detectMicroArch(info.Model, info.Arch, info.Features)
	return info, warnings
}

func readLinuxCPUFreqMHz(path string) float64 {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	value, err := strconv.ParseFloat(strings.TrimSpace(string(data)), 64)
	if err != nil || value <= 0 {
		return 0
	}
	// Linux cpufreq sysfs values are expressed in kHz, including
	// base_frequency, cpuinfo_max_freq, and scaling_max_freq.
	return value / 1000.0
}

func readLinuxCaches() map[string]int64 {
	out := defaultCacheSizes()
	entries, err := filepath.Glob("/sys/devices/system/cpu/cpu0/cache/index*")
	if err != nil {
		return out
	}
	for _, entry := range entries {
		levelData, err1 := os.ReadFile(filepath.Join(entry, "level"))
		typeData, err2 := os.ReadFile(filepath.Join(entry, "type"))
		sizeData, err3 := os.ReadFile(filepath.Join(entry, "size"))
		if err1 != nil || err2 != nil || err3 != nil {
			continue
		}
		level := strings.TrimSpace(string(levelData))
		cacheType := strings.ToUpper(strings.TrimSpace(string(typeData)))
		sizeBytes := parseLinuxSize(strings.TrimSpace(string(sizeData)))
		if sizeBytes == 0 {
			continue
		}
		key := "L" + level
		switch cacheType {
		case "DATA":
			key += "d"
		case "INSTRUCTION":
			key += "i"
		}
		out[key] = sizeBytes
	}
	return out
}

func parseLinuxSize(text string) int64 {
	text = strings.TrimSpace(strings.ToUpper(text))
	if text == "" {
		return 0
	}
	multiplier := int64(1)
	switch {
	case strings.HasSuffix(text, "K"):
		multiplier = 1 << 10
		text = strings.TrimSuffix(text, "K")
	case strings.HasSuffix(text, "M"):
		multiplier = 1 << 20
		text = strings.TrimSuffix(text, "M")
	}
	value, err := strconv.ParseInt(strings.TrimSpace(text), 10, 64)
	if err != nil {
		return 0
	}
	return value * multiplier
}

func countLinuxNUMANodes() int {
	nodes, err := filepath.Glob("/sys/devices/system/node/node*")
	if err != nil || len(nodes) == 0 {
		return 0
	}
	return len(nodes)
}
