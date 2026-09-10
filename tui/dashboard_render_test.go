package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/cloudapp3/vmbench/bench/netio"
	"github.com/cloudapp3/vmbench/i18n"
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

func TestDashboardSysCardShowsPublicIdentity(t *testing.T) {
	m := Model{sysInfo: evidenceSysInfo(), netIdent: netIdentState{
		v4: &netio.PublicIPIdentity{IP: "203.0.113.10", ASN: 64500, Org: "Example Net"},
		v6: &netio.PublicIPIdentity{IP: "2001:db8::1", ASN: 64500, ISP: "Example ISP"},
	}}
	view := dashboardSysCard(m, 60)
	// v6 carries ISP only, proving the Org→ISP fallback of the shared format.
	for _, want := range []string{"IPv4", "203.0.113.10", "AS64500 Example Net", "IPv6", "2001:db8::1", "AS64500 Example ISP"} {
		if !strings.Contains(view, want) {
			t.Fatalf("dashboard sys card missing %q:\n%s", want, view)
		}
	}
}

func TestDashboardSysCardShowsNetLoadingPlaceholder(t *testing.T) {
	m := Model{sysInfo: evidenceSysInfo(), netIdent: netIdentState{loading: true}}
	if view := dashboardSysCard(m, 60); !strings.Contains(view, i18n.T("tui.sys.netLoading")) {
		t.Fatalf("dashboard sys card missing identity loading placeholder:\n%s", view)
	}
}

func TestDashboardSysCardHidesNetRowsWhenUnreachable(t *testing.T) {
	// loading=false with nil identities is the offline terminal state.
	m := Model{sysInfo: evidenceSysInfo()}
	view := dashboardSysCard(m, 60)
	for _, banned := range []string{"IPv4", "IPv6", i18n.T("tui.sys.netLoading")} {
		if strings.Contains(view, banned) {
			t.Fatalf("dashboard sys card must hide unreachable identity rows, found %q:\n%s", banned, view)
		}
	}
}

func TestDashboardSysExpandedShowsCountryAndISP(t *testing.T) {
	m := Model{sysInfo: evidenceSysInfo(), netIdent: netIdentState{
		v4: &netio.PublicIPIdentity{IP: "203.0.113.10", Country: "United States", CountryCode: "US", ISP: "Example ISP"},
	}}
	view := dashboardSysExpanded(m, 60)
	if !strings.Contains(view, "US United States") || !strings.Contains(view, "Example ISP") {
		t.Fatalf("dashboard details card missing country/ISP rows:\n%s", view)
	}
}

// expandedSysInfo carries one populated value for every expanded evidence
// card so the full render path is exercised.
func expandedSysInfo() sysinfo.SystemInfo {
	info := evidenceSysInfo()
	info.CPU.Stepping = 1
	info.CPU.NumaNodes = 1
	info.Virtualization = sysinfo.VirtualizationInfo{System: "kvm", Role: "guest"}
	info.DMI = sysinfo.DMIInfo{ProductName: "Alibaba Cloud ECS", SysVendor: "Alibaba Cloud"}
	info.Memory.Type = "DDR4"
	info.Memory.FreqMHz = 2666
	info.Memory.Channels = 2
	info.Memory.UsedBytes = 3 << 30
	info.Memory.AvailableBytes = 12 << 30
	info.Memory.UsedPercent = 20
	info.Disks = []sysinfo.DiskInfo{{Device: "vda", Mountpoint: "/", FSType: "ext4", TotalBytes: 40 << 30}}
	info.Platform.NestedVirtualization = "vmx"
	info.Platform.KSM = "enabled"
	info.Platform.KSMPagesShared = 1234
	info.Platform.UptimeSeconds = 90061 // 1d 1h
	info.Platform.Load1, info.Platform.Load5, info.Platform.Load15 = 0.5, 0.4, 0.3
	info.Platform.SwapTotalBytes = 2 << 30
	info.Platform.SwapUsedBytes = 512 << 20
	return info
}

func TestDashboardSysExpandedShowsAllEvidenceCards(t *testing.T) {
	m := Model{sysInfo: expandedSysInfo()}
	view := dashboardSysExpanded(m, 120)
	for _, want := range []string{
		"Alibaba Cloud ECS", "kvm (guest)", "vmx", // virtualization
		"DDR4 2666 MT/s ×2", "20%", "12.0 GiB", // memory
		"vda", "ext4", "/", // storage
		"enabled (!)", "1234 pages", // oversell detail
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expanded evidence missing %q:\n%s", want, view)
		}
	}
	if loadRow := fmt.Sprintf("%.2f %.2f %.2f", 0.5, 0.4, 0.3); !strings.Contains(view, loadRow) {
		t.Fatalf("expanded evidence missing load row %q:\n%s", loadRow, view)
	}
	if !strings.Contains(view, i18n.Tf("tui.checkupSummary.daysHours", map[string]any{"Days": 1, "Hours": 1})) {
		t.Fatalf("expanded evidence missing uptime text:\n%s", view)
	}
}

func TestDashboardSysExpandedNarrowStillShowsAllCards(t *testing.T) {
	// Narrow terminals stack the cards vertically instead of pairing them.
	view := dashboardSysExpanded(Model{sysInfo: expandedSysInfo()}, 70)
	for _, want := range []string{"Alibaba Cloud ECS", "DDR4 2666 MT/s ×2", "vda"} {
		if !strings.Contains(view, want) {
			t.Fatalf("narrow expanded view missing %q:\n%s", want, view)
		}
	}
}

func TestDashboardSysExpandedOmitsEmptyEvidence(t *testing.T) {
	// evidenceSysInfo has no memory/disk/runtime evidence: those cards must
	// disappear while the oversell summary on the main card stays.
	view := dashboardSysExpanded(Model{sysInfo: evidenceSysInfo()}, 120)
	for _, banned := range []string{
		"kvm", "DDR4", "vda",
		i18n.T("tui.sys.uptime"), i18n.T("tui.sys.load"),
		i18n.T("tui.sys.swap"), i18n.T("tui.sys.nestedVirt"),
	} {
		if strings.Contains(view, banned) {
			t.Fatalf("expanded view must omit empty evidence, found %q:\n%s", banned, view)
		}
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
