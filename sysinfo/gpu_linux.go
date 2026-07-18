//go:build linux

package sysinfo

import (
	"context"
	"strings"
)

func collectGPUInfo(ctx context.Context) (*GPUInfo, []string) {
	output, err := runCommand(ctx, "sh", "-lc", "command -v lspci >/dev/null 2>&1 && lspci -mm | grep -Ei 'VGA|3D|Display' | head -n1")
	if err != nil || strings.TrimSpace(output) == "" {
		return nil, nil
	}
	return &GPUInfo{Model: strings.TrimSpace(output)}, nil
}
