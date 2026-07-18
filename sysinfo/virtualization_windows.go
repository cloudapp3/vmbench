//go:build windows

package sysinfo

import (
	"context"
	"strings"
)

func collectVirtualizationFallback(ctx context.Context) VirtualizationInfo {
	output, err := runCommand(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", "(Get-CimInstance Win32_ComputerSystem).Manufacturer + ' ' + (Get-CimInstance Win32_ComputerSystem).Model")
	if err != nil {
		return VirtualizationInfo{}
	}
	value := strings.ToLower(output)
	switch {
	case strings.Contains(value, "vmware"):
		return VirtualizationInfo{System: "vmware", Role: "guest"}
	case strings.Contains(value, "virtualbox"), strings.Contains(value, "innotek"):
		return VirtualizationInfo{System: "virtualbox", Role: "guest"}
	case strings.Contains(value, "microsoft") && strings.Contains(value, "virtual"):
		return VirtualizationInfo{System: "hyperv", Role: "guest"}
	case strings.Contains(value, "qemu"), strings.Contains(value, "kvm"):
		return VirtualizationInfo{System: "kvm", Role: "guest"}
	case strings.Contains(value, "parallels"):
		return VirtualizationInfo{System: "parallels", Role: "guest"}
	default:
		return VirtualizationInfo{}
	}
}
