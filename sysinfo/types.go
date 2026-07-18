package sysinfo

import (
	"context"
	"strings"

	gdisk "github.com/shirou/gopsutil/v4/disk"
	ghost "github.com/shirou/gopsutil/v4/host"
	gnet "github.com/shirou/gopsutil/v4/net"
)

// CPUInfo describes the detected CPU and topology.
type CPUInfo struct {
	Model         string           `json:"model"`
	Arch          string           `json:"arch"`
	PhysicalCores int              `json:"physical_cores"`
	LogicalCores  int              `json:"logical_cores"`
	BaseFreqMHz   float64          `json:"base_freq_mhz"`
	MaxFreqMHz    float64          `json:"max_freq_mhz"`
	CacheSizes    map[string]int64 `json:"cache_sizes"`
	Features      []string         `json:"features"`
	MicroArch     string           `json:"micro_arch"`
	NumaNodes     int              `json:"numa_nodes"`
}

// MemoryInfo describes the detected system memory.
type MemoryInfo struct {
	TotalBytes uint64 `json:"total_bytes"`
	Type       string `json:"type"`
	FreqMHz    int    `json:"freq_mhz"`
	Channels   int    `json:"channels"`
}

// OSInfo describes the detected operating system.
type OSInfo struct {
	Name      string `json:"name"`
	Kernel    string `json:"kernel"`
	GoVersion string `json:"go_version"`
	Hostname  string `json:"hostname"`
}

// GPUInfo describes the detected primary GPU, if available.
type GPUInfo struct {
	Model  string `json:"model"`
	VRAM   uint64 `json:"vram"`
	Driver string `json:"driver"`
}

// DiskInfo describes a mounted filesystem.
type DiskInfo struct {
	Device     string `json:"device"`
	Mountpoint string `json:"mountpoint"`
	FSType     string `json:"fs_type"`
	TotalBytes uint64 `json:"total_bytes"`
}

// NetworkInfo describes network reachability context for extension benches.
type NetworkInfo struct {
	InterfaceCount int      `json:"interface_count"`
	ActiveNames    []string `json:"active_names"`
}

// VirtualizationInfo describes locally detected virtualization context.
type VirtualizationInfo struct {
	System string `json:"system,omitempty"`
	Role   string `json:"role,omitempty"`
}

// SystemInfo aggregates all detected host information.
type SystemInfo struct {
	CPU            CPUInfo            `json:"cpu"`
	Memory         MemoryInfo         `json:"memory"`
	OS             OSInfo             `json:"os"`
	GPU            *GPUInfo           `json:"gpu,omitempty"`
	Disks          []DiskInfo         `json:"disks,omitempty"`
	Network        NetworkInfo        `json:"network"`
	Virtualization VirtualizationInfo `json:"virtualization"`
}

// Collect gathers the best-effort system information for the current host.
func Collect(ctx context.Context) (SystemInfo, []string) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return SystemInfo{}, []string{"sysinfo: " + err.Error()}
	}
	warnings := make([]string, 0, 8)

	cpuInfo, cpuWarnings := collectCPUInfo(ctx)
	warnings = append(warnings, cpuWarnings...)

	memoryInfo, memoryWarnings := collectMemoryInfo(ctx)
	warnings = append(warnings, memoryWarnings...)

	osInfo, osWarnings := collectOSInfo(ctx)
	warnings = append(warnings, osWarnings...)

	gpuInfo, gpuWarnings := collectGPUInfo(ctx)
	warnings = append(warnings, gpuWarnings...)

	disks, diskWarnings := collectDiskInfo(ctx)
	warnings = append(warnings, diskWarnings...)

	network, networkWarnings := collectNetworkInfo(ctx)
	warnings = append(warnings, networkWarnings...)

	virtualization, virtualizationWarnings := collectVirtualizationInfo(ctx)
	warnings = append(warnings, virtualizationWarnings...)

	return SystemInfo{
		CPU:            cpuInfo,
		Memory:         memoryInfo,
		OS:             osInfo,
		GPU:            gpuInfo,
		Disks:          disks,
		Network:        network,
		Virtualization: virtualization,
	}, compactWarnings(warnings)
}

func collectVirtualizationInfo(ctx context.Context) (VirtualizationInfo, []string) {
	return collectVirtualizationInfoWith(ctx, ghost.VirtualizationWithContext, collectVirtualizationFallback)
}

func collectVirtualizationInfoWith(
	ctx context.Context,
	primary func(context.Context) (string, string, error),
	fallback func(context.Context) VirtualizationInfo,
) (VirtualizationInfo, []string) {
	system, role, err := primary(ctx)
	info := VirtualizationInfo{
		System: strings.ToLower(strings.TrimSpace(system)),
		Role:   strings.ToLower(strings.TrimSpace(role)),
	}
	if (info.System == "" || info.Role == "") && fallback != nil {
		fallbackInfo := fallback(ctx)
		if info.System == "" {
			info.System = strings.ToLower(strings.TrimSpace(fallbackInfo.System))
		}
		if info.Role == "" {
			info.Role = strings.ToLower(strings.TrimSpace(fallbackInfo.Role))
		}
	}
	if err != nil && info.System == "" && info.Role == "" && !strings.Contains(strings.ToLower(err.Error()), "not implemented") {
		return info, []string{"virtualization: " + err.Error()}
	}
	return info, nil
}

func collectDiskInfo(ctx context.Context) ([]DiskInfo, []string) {
	parts, err := gdisk.PartitionsWithContext(ctx, false)
	if err != nil {
		return nil, []string{"disk: " + err.Error()}
	}

	seen := map[string]struct{}{}
	out := make([]DiskInfo, 0, len(parts))
	for _, part := range parts {
		key := strings.TrimSpace(part.Mountpoint)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}

		usage, err := gdisk.UsageWithContext(ctx, part.Mountpoint)
		if err != nil {
			out = append(out, DiskInfo{Device: part.Device, Mountpoint: part.Mountpoint, FSType: part.Fstype})
			continue
		}
		out = append(out, DiskInfo{
			Device:     part.Device,
			Mountpoint: part.Mountpoint,
			FSType:     part.Fstype,
			TotalBytes: usage.Total,
		})
	}
	return out, nil
}

func collectNetworkInfo(ctx context.Context) (NetworkInfo, []string) {
	interfaces, err := gnet.InterfacesWithContext(ctx)
	if err != nil {
		return NetworkInfo{}, []string{"network: " + err.Error()}
	}

	active := make([]string, 0, len(interfaces))
	for _, item := range interfaces {
		if len(item.Addrs) == 0 {
			continue
		}
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(item.Name)), "lo") {
			continue
		}
		active = append(active, item.Name)
	}
	return NetworkInfo{InterfaceCount: len(interfaces), ActiveNames: active}, nil
}

func compactWarnings(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, item := range in {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func hostPlatformName(ctx context.Context) string {
	info, err := ghost.InfoWithContext(ctx)
	if err != nil {
		return ""
	}
	platform := strings.TrimSpace(info.Platform)
	version := strings.TrimSpace(info.PlatformVersion)
	if platform == "" {
		return version
	}
	if version == "" {
		return platform
	}
	return platform + " " + version
}
