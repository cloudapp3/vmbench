//go:build !linux

package sysinfo

// nicHardware has no sysfs device backing off Linux; driver and PCI ids
// stay unknown, so PrimaryNIC renders empty and callers skip the line.
func nicHardware(names []string) (string, string) {
	return "", ""
}
