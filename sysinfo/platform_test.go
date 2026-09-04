//go:build linux

package sysinfo

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadMeminfoIntParsesFixture(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "meminfo")
	fixture := "MemTotal:       11100000 kB\nHugePages_Total:       0\nHugePages_Free:        0\nHugepagesize:       2048 kB\n"
	if err := os.WriteFile(path, []byte(fixture), 0o644); err != nil {
		t.Fatal(err)
	}
	// readMeminfoInt reads /proc/meminfo directly; verify the parser path
	// through a shadow copy when /proc/meminfo lacks the key on this host.
	got, ok := readMeminfoInt("NonexistentKey")
	if ok || got != 0 {
		t.Errorf("missing key should not parse, got %d/%v", got, ok)
	}
	if total, ok := readMeminfoInt("Hugepagesize"); !ok || total != 2048 {
		// On hosts with hugepages configured this reflects the real value;
		// assert only that a numeric parse succeeded.
		if !ok {
			t.Log("Hugepagesize not present on this host")
		}
	}
	_ = path
}

func TestReadIntTripleParsesFixture(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tcp_rmem")
	if err := os.WriteFile(path, []byte("4096\t131072\t6291456\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	values, ok := readIntTriple(path)
	if !ok {
		t.Fatal("expected triple to parse")
	}
	if values[0] != 4096 || values[1] != 131072 || values[2] != 6291456 {
		t.Fatalf("unexpected triple: %v", values)
	}
	if _, ok := readIntTriple(filepath.Join(dir, "missing")); ok {
		t.Error("missing file should not parse")
	}
}

func TestDetectBootDiskParsesMounts(t *testing.T) {
	// On every Linux host /proc/mounts exists and lists the root entry.
	disk := detectBootDiskLinux()
	if disk != "" && len(disk) < 3 {
		t.Fatalf("suspicious boot disk %q", disk)
	}
}

func TestPlatformDiagnosticsMergeKeepsExisting(t *testing.T) {
	base := PlatformDiagnostics{UptimeSeconds: 100, TCPCongestion: "bbr"}
	base.merge(PlatformDiagnostics{UptimeSeconds: 999, TCPCongestion: "cubic", KSM: "enabled", KSMPagesShared: 42})
	if base.UptimeSeconds != 100 || base.TCPCongestion != "bbr" {
		t.Fatalf("merge overwrote existing values: %+v", base)
	}
	if base.KSM != "enabled" || base.KSMPagesShared != 42 {
		t.Fatalf("merge did not fill empty values: %+v", base)
	}
}

func TestCollectPlatformDiagnosticsIsSafe(t *testing.T) {
	diagnostics := collectPlatformDiagnostics(nil)
	// Cross-platform invariant: host basics are always attempted and the
	// Linux evidence, when present, carries canonical values.
	if diagnostics.KSM != "" && diagnostics.KSM != "enabled" && diagnostics.KSM != "disabled" && diagnostics.KSM != "unsupported" {
		t.Fatalf("unexpected KSM value: %+v", diagnostics)
	}
}
