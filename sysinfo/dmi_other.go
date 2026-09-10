//go:build !linux

package sysinfo

import "context"

// collectDMIInfo is a no-op off Linux; the DMI identity fields stay empty
// and renderers skip them.
func collectDMIInfo(ctx context.Context) DMIInfo {
	return DMIInfo{}
}
