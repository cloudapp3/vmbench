//go:build linux

package sysinfo

import (
	"context"
	"os"
	"strings"
)

func collectVirtualizationFallback(ctx context.Context) VirtualizationInfo {
	if output, err := runCommand(ctx, "systemd-detect-virt"); err == nil {
		if system := normalizeVirtualizationSystem(output); system != "" && system != "none" {
			return VirtualizationInfo{System: system, Role: "guest"}
		}
	}

	values := make([]string, 0, 3)
	for _, path := range []string{"/sys/class/dmi/id/product_name", "/sys/class/dmi/id/sys_vendor", "/sys/class/dmi/id/board_vendor"} {
		if data, err := os.ReadFile(path); err == nil {
			values = append(values, string(data))
		}
	}
	if system := virtualizationSystemFromText(strings.Join(values, " ")); system != "" {
		return VirtualizationInfo{System: system, Role: "guest"}
	}

	if _, err := os.Stat("/.dockerenv"); err == nil {
		return VirtualizationInfo{System: "docker", Role: "guest"}
	}
	if data, err := os.ReadFile("/proc/1/cgroup"); err == nil {
		if system := virtualizationSystemFromText(string(data)); system == "docker" || system == "lxc" || system == "containerd" {
			return VirtualizationInfo{System: system, Role: "guest"}
		}
	}
	return VirtualizationInfo{}
}

func normalizeVirtualizationSystem(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "microsoft":
		return "hyperv"
	case "oracle":
		return "virtualbox"
	default:
		return value
	}
}

func virtualizationSystemFromText(value string) string {
	value = strings.ToLower(value)
	tests := []struct {
		needle string
		system string
	}{
		{needle: "docker", system: "docker"},
		{needle: "containerd", system: "containerd"},
		{needle: "lxc", system: "lxc"},
		{needle: "openvz", system: "openvz"},
		{needle: "vmware", system: "vmware"},
		{needle: "virtualbox", system: "virtualbox"},
		{needle: "innotek", system: "virtualbox"},
		{needle: "microsoft", system: "hyperv"},
		{needle: "hyper-v", system: "hyperv"},
		{needle: "xen", system: "xen"},
		{needle: "qemu", system: "qemu"},
		{needle: "kvm", system: "kvm"},
		{needle: "parallels", system: "parallels"},
		{needle: "bhyve", system: "bhyve"},
	}
	for _, test := range tests {
		if strings.Contains(value, test.needle) {
			return test.system
		}
	}
	return ""
}
