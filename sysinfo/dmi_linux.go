//go:build linux

package sysinfo

import (
	"context"
	"os"
	"path/filepath"
	"strings"
)

// dmiSysfsFiles maps SystemInfo.DMI fields to their sysfs attributes.
var dmiSysfsFiles = []struct {
	attr  string
	field func(*DMIInfo) *string
}{
	{"product_name", func(d *DMIInfo) *string { return &d.ProductName }},
	{"sys_vendor", func(d *DMIInfo) *string { return &d.SysVendor }},
	{"board_vendor", func(d *DMIInfo) *string { return &d.BoardVendor }},
	{"board_name", func(d *DMIInfo) *string { return &d.BoardName }},
}

// collectDMIInfo reads SMBIOS identity strings from sysfs. Missing DMI is
// normal (containers, some hypervisors), so failures stay silent.
func collectDMIInfo(ctx context.Context) DMIInfo {
	return readDMIInfoAt("/sys/class/dmi/id")
}

// readDMIInfoAt reads the DMI attributes from a sysfs-style directory so
// tests can point it at a fixture tree.
func readDMIInfoAt(dir string) DMIInfo {
	var out DMIInfo
	for _, item := range dmiSysfsFiles {
		data, err := os.ReadFile(filepath.Join(dir, item.attr))
		if err != nil {
			continue
		}
		if value := cleanDMIValue(string(data)); value != "" {
			*item.field(&out) = value
		}
	}
	return out
}

// junkDMIValues lists filler strings some vendors bake into unpopulated
// SMBIOS fields; they carry no identity evidence.
var junkDMIValues = map[string]struct{}{
	"to be filled by o.e.m.": {},
	"default string":         {},
	"default name":           {},
	"system product name":    {},
	"system manufacturer":    {},
	"system serial number":   {},
	"to be defined by oem":   {},
	"none":                   {},
	"not specified":          {},
	"unknown":                {},
	"empty":                  {},
}

func cleanDMIValue(raw string) string {
	value := strings.TrimSpace(strings.TrimRight(raw, "\n"))
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if _, junk := junkDMIValues[strings.ToLower(value)]; junk {
		return ""
	}
	return value
}
