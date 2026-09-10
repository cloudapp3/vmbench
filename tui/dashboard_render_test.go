package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	gbreport "github.com/cloudapp3/vmbench/report"
	"github.com/cloudapp3/vmbench/sysinfo"
)

func evidenceSysInfo() sysinfo.SystemInfo {
	return sysinfo.SystemInfo{
		CPU: sysinfo.CPUInfo{
			Model: "Evidence CPU", PhysicalCores: 2, LogicalCores: 4,
			CacheSizes: map[string]int64{"L1d": 32 << 10, "L2": 4 << 20, "L3": 16 << 20},
		},
		Network: sysinfo.NetworkInfo{PrimaryDriver: "virtio_net", PrimaryPCI: "1af4:1000"},
		Platform: sysinfo.PlatformDiagnostics{
			VirtioBalloon: "present",
			KSM:           "disabled",
		},
	}
}

func TestDashboardSysCardShowsNICAndOversell(t *testing.T) {
	m := Model{sysInfo: evidenceSysInfo()}
	view := dashboardSysCard(m, 60)
	for _, want := range []string{"virtio_net (1af4:1000)", "balloon=present (!)", "ksm=disabled"} {
		if !strings.Contains(view, want) {
			t.Fatalf("dashboard sys card missing %q:\n%s", want, view)
		}
	}
}

func TestDashboardSysExpandedShowsCacheLine(t *testing.T) {
	m := Model{sysInfo: evidenceSysInfo()}
	view := dashboardSysExpanded(m, 60)
	if !strings.Contains(view, "L1d 32 KiB") || !strings.Contains(view, "L3 16 MiB") {
		t.Fatalf("dashboard details card missing cache line:\n%s", view)
	}
}

func TestDashboardSysCardOmitsAbsentEvidence(t *testing.T) {
	view := dashboardSysCard(Model{}, 60)
	for _, banned := range []string{"virtio", "balloon", "ksm"} {
		if strings.Contains(view, banned) {
			t.Fatalf("dashboard sys card must omit absent evidence, found %q:\n%s", banned, view)
		}
	}
	if view := dashboardSysExpanded(Model{}, 60); strings.Contains(view, "L1d") {
		t.Fatalf("dashboard details card must omit absent cache line:\n%s", view)
	}
}

func TestCompareSysCardShowsOversellAndNIC(t *testing.T) {
	doc := gbreport.Document{System: evidenceSysInfo()}
	view := compareSysCard("A", "report-a.json", doc, 60, lipgloss.AdaptiveColor{Light: "#3b82f6", Dark: "#60a5fa"})
	for _, want := range []string{"virtio_net (1af4:1000)", "balloon=present (!)"} {
		if !strings.Contains(view, want) {
			t.Fatalf("compare sys card missing %q:\n%s", want, view)
		}
	}
}
