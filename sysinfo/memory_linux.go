//go:build linux

package sysinfo

import (
	"context"

	gmem "github.com/shirou/gopsutil/v4/mem"
)

func collectMemoryInfo(ctx context.Context) (MemoryInfo, []string) {
	vm, err := gmem.VirtualMemoryWithContext(ctx)
	if err != nil {
		return MemoryInfo{}, []string{"memory: " + err.Error()}
	}
	return MemoryInfo{TotalBytes: vm.Total}, nil
}
