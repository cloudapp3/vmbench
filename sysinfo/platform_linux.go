//go:build linux

package sysinfo

import (
	"os"
	"strconv"
	"strings"
)

// collectLinuxKernelTuning reads kernel/virtualization evidence from /proc
// and /sys. Every read is best-effort: missing files (containers, older
// kernels, masked paths) leave the field empty.
func collectLinuxKernelTuning() PlatformDiagnostics {
	diagnostics := PlatformDiagnostics{}

	if _, err := os.Stat("/sys/bus/virtio/drivers/virtio_balloon"); err == nil {
		diagnostics.VirtioBalloon = "present"
	} else {
		diagnostics.VirtioBalloon = "absent"
	}

	if run, ok := readIntFile("/sys/kernel/mm/ksm/run"); ok {
		if run == 1 {
			diagnostics.KSM = "enabled"
			diagnostics.KSMPagesShared, _ = readIntFile("/sys/kernel/mm/ksm/pages_shared")
		} else {
			diagnostics.KSM = "disabled"
		}
	} else {
		diagnostics.KSM = "unsupported"
	}

	diagnostics.TCPCongestion, _ = readStringFile("/proc/sys/net/ipv4/tcp_congestion_control")
	diagnostics.TCPQDisc, _ = readStringFile("/proc/sys/net/core/default_qdisc")

	if values, ok := readIntTriple("/proc/sys/net/ipv4/tcp_rmem"); ok {
		diagnostics.TCPRmemMin, diagnostics.TCPRmemDefault, diagnostics.TCPRmemMax = values[0], values[1], values[2]
	}
	if values, ok := readIntTriple("/proc/sys/net/ipv4/tcp_wmem"); ok {
		diagnostics.TCPWmemMin, diagnostics.TCPWmemDefault, diagnostics.TCPWmemMax = values[0], values[1], values[2]
	}

	diagnostics.NestedVirtualization = detectNestedVirtualizationLinux()

	diagnostics.HugePagesTotal, _ = readMeminfoInt("HugePages_Total")
	diagnostics.HugePagesFree, _ = readMeminfoInt("HugePages_Free")
	if kb, ok := readMeminfoInt("Hugepagesize"); ok {
		diagnostics.HugePageSizeBytes = kb * 1024
	}

	diagnostics.BootDisk = detectBootDiskLinux()
	diagnostics.normalize()
	return diagnostics
}

// detectNestedVirtualizationLinux reads CPU flags from /proc/cpuinfo. Inside
// containers the flags may reflect the host or be masked; absence is reported
// as "masked" rather than a definitive "unsupported".
func detectNestedVirtualizationLinux() string {
	data, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return "unknown"
	}
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "flags") && !strings.HasPrefix(line, "Features") {
			continue
		}
		flags := strings.Fields(line)
		for _, flag := range flags {
			switch flag {
			case "vmx":
				return "vmx"
			case "svm":
				return "svm"
			}
		}
		return "masked"
	}
	return "unknown"
}

// detectBootDiskLinux returns the device backing the root filesystem.
func detectBootDiskLinux() string {
	data, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return ""
	}
	var rootDevice, bootDevice string
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		device, mountpoint := fields[0], fields[1]
		if !strings.HasPrefix(device, "/dev/") {
			continue
		}
		switch mountpoint {
		case "/":
			if rootDevice == "" {
				rootDevice = device
			}
		case "/boot":
			bootDevice = device
		}
	}
	if bootDevice != "" {
		return bootDevice
	}
	return rootDevice
}

func readStringFile(path string) (string, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	value := strings.TrimSpace(string(data))
	return value, value != ""
}

func readIntFile(path string) (int64, bool) {
	value, ok := readStringFile(path)
	if !ok {
		return 0, false
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, false
	}
	return parsed, true
}

func readIntTriple(path string) ([3]int64, bool) {
	value, ok := readStringFile(path)
	if !ok {
		return [3]int64{}, false
	}
	fields := strings.Fields(value)
	if len(fields) < 3 {
		return [3]int64{}, false
	}
	var out [3]int64
	for i := 0; i < 3; i++ {
		parsed, err := strconv.ParseInt(fields[i], 10, 64)
		if err != nil {
			return [3]int64{}, false
		}
		out[i] = parsed
	}
	return out, true
}

func readMeminfoInt(key string) (int64, bool) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, false
	}
	prefix := key + ":"
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		fields := strings.Fields(strings.TrimPrefix(line, prefix))
		if len(fields) == 0 {
			return 0, false
		}
		parsed, err := strconv.ParseInt(fields[0], 10, 64)
		if err != nil {
			return 0, false
		}
		return parsed, true
	}
	return 0, false
}
