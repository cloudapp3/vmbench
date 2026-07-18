//go:build darwin

package sysinfo

import (
	"context"
	"strings"
)

func collectVirtualizationFallback(ctx context.Context) VirtualizationInfo {
	value, err := runCommand(ctx, "sysctl", "-n", "kern.hv_vmm_present")
	if err == nil && strings.TrimSpace(value) == "1" {
		return VirtualizationInfo{System: "apple_hypervisor", Role: "guest"}
	}
	return VirtualizationInfo{}
}
