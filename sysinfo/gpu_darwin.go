//go:build darwin

package sysinfo

import (
	"context"
	"strings"
)

func collectGPUInfo(ctx context.Context) (*GPUInfo, []string) {
	output, err := runCommand(ctx, "system_profiler", "SPDisplaysDataType")
	if err != nil || strings.TrimSpace(output) == "" {
		return nil, nil
	}
	var model string
	var vram uint64
	for _, line := range splitLines(output) {
		switch {
		case strings.Contains(line, "Chipset Model:"):
			model = strings.TrimSpace(strings.TrimPrefix(line, "Chipset Model:"))
		case strings.Contains(line, "VRAM") && vram == 0:
			vram = parseVRAMBytes(line)
		}
	}
	if model == "" {
		return nil, nil
	}
	return &GPUInfo{Model: model, VRAM: vram}, nil
}
