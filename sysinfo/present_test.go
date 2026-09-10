package sysinfo

import (
	"strings"
	"testing"
)

func TestOversellSignalsCarryRawState(t *testing.T) {
	d := PlatformDiagnostics{VirtioBalloon: "present", KSM: "disabled"}
	signals := d.OversellSignals()
	if len(signals) != 2 {
		t.Fatalf("expected 2 signals, got %d", len(signals))
	}
	for _, signal := range signals {
		if signal.State == "" {
			t.Fatalf("signal %q missing raw state", signal.Key)
		}
		if signal.On != (signal.State == "present" || signal.State == "enabled") {
			t.Fatalf("signal %q state %q disagrees with On=%v", signal.Key, signal.State, signal.On)
		}
		if signal.Risk != signal.On {
			t.Fatalf("signal %q state %q: Risk must follow On", signal.Key, signal.State)
		}
	}

	// Unsupported/unknown evidence is omitted: bare metal shows nothing.
	if got := (PlatformDiagnostics{VirtioBalloon: "unsupported", KSM: ""}).OversellSignals(); len(got) != 0 {
		t.Fatalf("unsupported evidence must yield no signals, got %v", got)
	}
}

func TestPrimaryNICDisplayText(t *testing.T) {
	cases := []struct {
		name string
		nic  NetworkInfo
		want string
	}{
		{"driver and pci", NetworkInfo{PrimaryDriver: "virtio_net", PrimaryPCI: "1af4:1000"}, "virtio_net (1af4:1000)"},
		{"driver only", NetworkInfo{PrimaryDriver: "virtio_net"}, "virtio_net"},
		{"pci only", NetworkInfo{PrimaryPCI: "1af4:1000"}, "1af4:1000"},
		{"none", NetworkInfo{}, ""},
		{"padding trimmed", NetworkInfo{PrimaryDriver: "  virtio_net  ", PrimaryPCI: " 1af4:1000 "}, "virtio_net (1af4:1000)"},
	}
	for _, tc := range cases {
		if got := tc.nic.PrimaryNIC(); got != tc.want {
			t.Errorf("%s: PrimaryNIC() = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestFormatCacheLineOrdersLevels(t *testing.T) {
	// Well-known levels render in fixed order regardless of map iteration.
	got := FormatCacheLine(map[string]int64{"L3": 16 << 20, "L1d": 32 << 10, "L2": 4 << 20, "L1i": 48 << 10})
	want := "L1d 32 KiB, L1i 48 KiB, L2 4 MiB, L3 16 MiB"
	if got != want {
		t.Fatalf("FormatCacheLine() = %q, want %q", got, want)
	}
	if got := FormatCacheLine(nil); got != "" {
		t.Fatalf("empty cache map must render empty, got %q", got)
	}
	if !strings.Contains(FormatCacheLine(map[string]int64{"L3": 3<<20 + 512<<10}), "3.5 MiB") {
		t.Fatalf("fractional MiB rendering broken")
	}
}
