//go:build linux

package sysinfo

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadLinuxCPUFreqMHzConvertsKHz(t *testing.T) {
	path := filepath.Join(t.TempDir(), "base_frequency")
	if err := os.WriteFile(path, []byte("2400000\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := readLinuxCPUFreqMHz(path); got != 2400 {
		t.Fatalf("frequency = %f MHz, want 2400", got)
	}
}
