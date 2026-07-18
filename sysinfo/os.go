package sysinfo

import (
	"os"
	"runtime"
	"strings"
)

func buildOSInfo(name, kernel string) (OSInfo, []string) {
	hostname, err := os.Hostname()
	warnings := make([]string, 0, 1)
	if err != nil {
		warnings = append(warnings, "hostname: "+err.Error())
	}
	return OSInfo{
		Name:      strings.TrimSpace(name),
		Kernel:    strings.TrimSpace(kernel),
		GoVersion: runtime.Version(),
		Hostname:  strings.TrimSpace(hostname),
	}, warnings
}
