//go:build windows

package sysinfo

import (
	"context"
	"encoding/json"
	"strings"

	gmem "github.com/shirou/gopsutil/v4/mem"
)

type winMemoryProbe struct {
	SMBIOSMemoryType int    `json:"SMBIOSMemoryType"`
	Speed            uint32 `json:"Speed"`
}

func collectMemoryInfo(ctx context.Context) (MemoryInfo, []string) {
	vm, err := gmem.VirtualMemoryWithContext(ctx)
	if err != nil {
		return MemoryInfo{}, []string{"memory: " + err.Error()}
	}
	out := MemoryInfo{TotalBytes: vm.Total}
	text, cmdErr := runCommand(ctx, "powershell", "-NoProfile", "-Command", "Get-CimInstance Win32_PhysicalMemory | Select-Object -First 1 SMBIOSMemoryType,Speed | ConvertTo-Json -Compress")
	if cmdErr == nil && strings.TrimSpace(text) != "" {
		var probe winMemoryProbe
		if err := json.Unmarshal([]byte(text), &probe); err == nil {
			out.FreqMHz = int(probe.Speed)
			switch probe.SMBIOSMemoryType {
			case 26:
				out.Type = "DDR4"
			case 34:
				out.Type = "DDR5"
			}
		}
	}
	return out, nil
}
