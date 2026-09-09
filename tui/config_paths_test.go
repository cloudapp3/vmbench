package tui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigPathsUsesPlatformDir(t *testing.T) {
	base := t.TempDir()
	t.Setenv("VMBENCH_CONFIG", "")
	t.Setenv("XDG_CONFIG_HOME", base)
	file, dir := ConfigPaths()
	if want := filepath.Join(base, "vmbench", "config.json"); file != want {
		t.Fatalf("ConfigPaths file = %q, want %q", file, want)
	}
	if want := filepath.Join(base, "vmbench"); dir != want {
		t.Fatalf("ConfigPaths dir = %q, want %q", dir, want)
	}
}

// TestConfigPathsRedirectedKeepsFileOnly pins the contract uninstall relies
// on: a VMBENCH_CONFIG override makes only the file vmbench-owned, so the
// directory must come back empty.
func TestConfigPathsRedirectedKeepsFileOnly(t *testing.T) {
	file := filepath.Join(t.TempDir(), "custom-config.json")
	t.Setenv("VMBENCH_CONFIG", file)
	gotFile, dir := ConfigPaths()
	if gotFile != file {
		t.Fatalf("ConfigPaths file = %q, want %q", gotFile, file)
	}
	if dir != "" {
		t.Fatalf("ConfigPaths dir = %q, want empty for redirected config", dir)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatalf("ConfigPaths must not create %s", file)
	}
}
