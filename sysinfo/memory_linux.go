//go:build linux

package sysinfo

import (
	"context"
	"strconv"
	"strings"

	gmem "github.com/shirou/gopsutil/v4/mem"
)

func collectMemoryInfo(ctx context.Context) (MemoryInfo, []string) {
	vm, err := gmem.VirtualMemoryWithContext(ctx)
	if err != nil {
		return MemoryInfo{}, []string{"memory: " + err.Error()}
	}
	out := MemoryInfo{
		TotalBytes:     vm.Total,
		UsedBytes:      vm.Used,
		AvailableBytes: vm.Available,
		UsedPercent:    vm.UsedPercent,
	}
	// dmidecode needs root; unprivileged runs simply keep the fields empty
	// (zero means unknown, never "no memory modules"). Silent like the ping
	// ICMP fallback: absence of evidence is not a warning.
	if text, err := runCommand(ctx, "dmidecode", "-t", "17"); err == nil {
		dmi := parseDMIDecodeMemory(text)
		if out.Type == "" {
			out.Type = dmi.Type
		}
		if out.FreqMHz == 0 {
			out.FreqMHz = dmi.FreqMHz
		}
		if out.Channels == 0 {
			out.Channels = dmi.Channels
		}
	}
	return out, nil
}

// dmidecodeMemory holds the populated-module evidence parsed from
// `dmidecode -t 17` output.
type dmidecodeMemory struct {
	Type     string
	FreqMHz  int
	Channels int
}

// parseDMIDecodeMemory scans Memory Device records for module type, speed,
// and the count of populated slots. dmidecode indents record fields, so a
// non-indented "Memory Device" line starts a record and indented "Key: value"
// lines fill it.
func parseDMIDecodeMemory(text string) dmidecodeMemory {
	var out dmidecodeMemory
	inDevice := false
	for _, raw := range strings.Split(text, "\n") {
		indented := strings.HasPrefix(raw, " ") || strings.HasPrefix(raw, "\t")
		line := strings.TrimSpace(raw)
		if !indented {
			inDevice = line == "Memory Device"
			continue
		}
		if !inDevice {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		switch strings.TrimSpace(key) {
		case "Size":
			if value != "" && !strings.HasPrefix(value, "No Module") {
				out.Channels++
			}
		case "Type":
			if out.Type == "" && validDMIType(value) {
				out.Type = value
			}
		case "Speed":
			if out.FreqMHz == 0 {
				out.FreqMHz = parseDMISpeedMHz(value)
			}
		}
	}
	return out
}

// validDMIType keeps DDR generations; dmidecode reports "Unknown", "Other",
// or blank for unpopulated or virtual slots.
func validDMIType(value string) bool {
	return strings.Contains(value, "DDR") && !strings.Contains(value, "Unknown") && !strings.Contains(value, "Other")
}

// parseDMISpeedMHz extracts the leading rate ("2666" from "2666 MT/s" or
// "3200 MHz"); the SMBIOS speed and its unit share the same numeric scale.
func parseDMISpeedMHz(value string) int {
	fields := strings.Fields(value)
	if len(fields) == 0 {
		return 0
	}
	rate, err := strconv.Atoi(fields[0])
	if err != nil || rate <= 0 {
		return 0
	}
	return rate
}
