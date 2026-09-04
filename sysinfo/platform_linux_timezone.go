//go:build linux

package sysinfo

import (
	"context"
	"os"
	"strings"
	"time"
)

// currentTimezone prefers the explicit system timezone configuration and
// falls back to the local time zone name.
func currentTimezone(ctx context.Context) (string, error) {
	if zone := readLocaltimeZone(); zone != "" {
		return zone, nil
	}
	name, _ := time.Now().Zone()
	return name, nil
}

// readLocaltimeZone resolves /etc/localtime (usually a symlink into
// /usr/share/zoneinfo) to an IANA timezone name.
func readLocaltimeZone() string {
	target, err := os.Readlink("/etc/localtime")
	if err != nil {
		return ""
	}
	marker := "zoneinfo/"
	index := strings.LastIndex(target, marker)
	if index < 0 {
		return ""
	}
	zone := strings.TrimSpace(target[index+len(marker):])
	return strings.TrimSuffix(zone, "/")
}
