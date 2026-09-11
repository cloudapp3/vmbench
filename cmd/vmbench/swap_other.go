//go:build !linux

package main

import "regexp"

// maybeSetupTempSwap is a no-op off Linux: mkswap/swapon have no portable
// equivalent on macOS or Windows.
func maybeSetupTempSwap(bf *benchmarkFlags, tools []string, filter *regexp.Regexp) func() {
	return func() {}
}
