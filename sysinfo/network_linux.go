//go:build linux

package sysinfo

import (
	"os"
	"path/filepath"
	"strings"
)

// nicHardware resolves the kernel driver and PCI vendor:device id of the
// first interface backed by a physical device. lo and software-only
// interfaces have no sysfs device backing, so they are skipped. Every read
// is best-effort; empty means unknown.
func nicHardware(names []string) (string, string) {
	var driver, pci string
	for _, name := range names {
		device := filepath.Join("/sys/class/net", name, "device")
		if _, err := os.Stat(device); err != nil {
			continue
		}
		if target, err := os.Readlink(filepath.Join(device, "driver")); err == nil {
			driver = filepath.Base(strings.TrimRight(target, "/"))
		}
		vendor, hasVendor := readSysText(filepath.Join(device, "vendor"))
		devID, hasDevID := readSysText(filepath.Join(device, "device"))
		if hasVendor && hasDevID {
			pci = strings.TrimPrefix(vendor, "0x") + ":" + strings.TrimPrefix(devID, "0x")
		}
		if driver != "" || pci != "" {
			return driver, pci
		}
	}
	return driver, pci
}

// readSysText reads a small sysfs text value, returning ("", false) when the
// file is missing or unreadable.
func readSysText(path string) (string, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	value := strings.TrimSpace(string(data))
	return value, value != ""
}
