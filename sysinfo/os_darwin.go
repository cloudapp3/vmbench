//go:build darwin

package sysinfo

import (
	"context"
	"strings"
)

func collectOSInfo(ctx context.Context) (OSInfo, []string) {
	productName, _ := runCommand(ctx, "sw_vers", "-productName")
	productVersion, _ := runCommand(ctx, "sw_vers", "-productVersion")
	kernel, _ := runCommand(ctx, "uname", "-r")
	name := strings.TrimSpace(strings.TrimSpace(productName) + " " + strings.TrimSpace(productVersion))
	return buildOSInfo(name, kernel)
}
