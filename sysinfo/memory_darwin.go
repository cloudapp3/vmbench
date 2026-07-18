//go:build darwin

package sysinfo

import (
	"context"
	"strconv"
	"strings"

	gmem "github.com/shirou/gopsutil/v4/mem"
)

func collectMemoryInfo(ctx context.Context) (MemoryInfo, []string) {
	vm, err := gmem.VirtualMemoryWithContext(ctx)
	if err != nil {
		return MemoryInfo{}, []string{"memory: " + err.Error()}
	}
	out := MemoryInfo{TotalBytes: vm.Total}
	text, sysctlErr := runCommand(ctx, "sysctl", "-n", "hw.memsize")
	if sysctlErr == nil {
		if value, parseErr := strconv.ParseUint(strings.TrimSpace(text), 10, 64); parseErr == nil && value > 0 {
			out.TotalBytes = value
		}
	}
	return out, nil
}
