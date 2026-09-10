package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/cloudapp3/vmbench/bench/netio"
	"github.com/cloudapp3/vmbench/checkup"
	"github.com/cloudapp3/vmbench/i18n"
	"github.com/cloudapp3/vmbench/sysinfo"
)

func TestCheckupPagesFitCompactTerminalWidth(t *testing.T) {
	const width = 80
	info := sysinfo.SystemInfo{CPU: sysinfo.CPUInfo{
		Model:         "AMD EPYC 9754 128-Core Processor",
		PhysicalCores: 128,
		LogicalCores:  256,
	}}
	report := checkup.CheckupReport{
		Status:  "failed",
		Message: "0/1 sections ok; failed: speed",
		Speed: checkup.SpeedSection{
			SectionState: checkup.SectionState{Enabled: true, Status: "error"},
			Result: &checkup.SpeedResult{Groups: []checkup.SpeedProviderGroup{{
				Provider:      "iperf3",
				ProviderLabel: "iperf3",
				Status:        "error",
			}}},
		},
	}

	tests := []struct {
		name  string
		page  page
		setup func(*Model)
	}{
		{name: "config", page: pageConfig},
		{name: "running", page: pageRunning, setup: func(m *Model) {
			m.runKind = "checkup"
			m.checkupSections = []checkupSection{
				{id: checkup.SectionHardware, label: "Hardware", status: "done", message: "ok"},
				{id: checkup.SectionNetworkInfo, label: "Network Info", status: "done", message: "ok"},
				{id: checkup.SectionRoute, label: "Route", status: "done", message: "ok"},
				{id: checkup.SectionPing, label: "Ping", status: "running"},
				{id: checkup.SectionSpeed, label: "Speed", status: "waiting"},
				{id: checkup.SectionIPQuality, label: "IP Quality", status: "waiting"},
				{id: checkup.SectionReachability, label: "Reachability", status: "waiting"},
				{id: checkup.SectionMail, label: "Mail Ports", status: "skip"},
			}
		}},
		{name: "results", page: pageCheckupResults, setup: func(m *Model) { m.checkupReport = &report }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewModel("", "")
			m.page = tt.page
			m.width = width
			m.height = 60
			m.sysInfo = info
			if tt.setup != nil {
				tt.setup(&m)
			}

			for lineNumber, line := range strings.Split(m.View(), "\n") {
				if got := lipgloss.Width(line); got > width {
					t.Fatalf("line %d width = %d, want <= %d", lineNumber+1, got, width)
				}
			}
		})
	}
}

func TestRouteResultCardShowsPartialDestinationStatus(t *testing.T) {
	notReached := false
	report := checkup.CheckupReport{Route: checkup.RouteSection{
		SectionState: checkup.SectionState{Enabled: true, Status: "partial"},
		Results: []checkup.RouteRun{{
			Target:             netio.TraceTarget{City: "Chengdu", Carrier: "CERNET"},
			DestinationReached: &notReached,
			Status:             netio.TraceStatusPartial,
			Hops:               []netio.Hop{{TTL: 1, IP: "192.0.2.1"}},
		}},
	}}
	view := routeResultCard(report, 80)
	if !strings.Contains(view, "PARTIAL") {
		t.Fatalf("route result card did not show partial status:\n%s", view)
	}
}

func TestRouteResultCardShowsLineLabel(t *testing.T) {
	reached := true
	report := checkup.CheckupReport{Route: checkup.RouteSection{
		SectionState: checkup.SectionState{Enabled: true, Status: "ok"},
		Results: []checkup.RouteRun{{
			Target:             netio.TraceTarget{Name: "广州电信", City: "Guangzhou", Carrier: "CT"},
			ResolvedTarget:     "202.96.209.133",
			DestinationReached: &reached,
			Status:             netio.TraceStatusOK,
			Classification: &netio.RouteClassification{
				Code: "ct_cn2_gia", Label: "电信CN2GIA [精品线路]", Confidence: "confirmed", Rank: 5,
			},
		}},
	}}
	view := routeResultCard(report, 80)
	for _, want := range []string{"广州电信", "202.96.209.133", "电信CN2GIA [精品线路]"} {
		if !strings.Contains(view, want) {
			t.Fatalf("route result card missing %q:\n%s", want, view)
		}
	}
}

func TestPingResultCardShowsRefusedConnectionState(t *testing.T) {
	report := checkup.CheckupReport{Ping: checkup.PingSection{
		SectionState: checkup.SectionState{Enabled: true, Status: "ok"},
		Results: []checkup.PingResult{{
			Name:            "Closed port",
			Status:          "ok",
			ConnectionState: netio.PingConnectionStateRefused,
			Sent:            10,
			Received:        10,
		}},
	}}
	view := pingResultCard(report, 80)
	if !strings.Contains(view, "refused") {
		t.Fatalf("ping result card did not show refused state:\n%s", view)
	}
}

func TestPingResultCardShowsICMPFallbackMarker(t *testing.T) {
	report := checkup.CheckupReport{Ping: checkup.PingSection{
		SectionState: checkup.SectionState{Enabled: true, Status: "ok"},
		Results: []checkup.PingResult{{
			Name:          "GZ CT CN2",
			Status:        "ok",
			ProbeProtocol: "icmp-echo",
			ProbeTool:     "ping",
			AvgLatencyMs:  10.9,
			Sent:          10,
			Received:      10,
		}},
	}}
	view := pingResultCard(report, 80)
	if !strings.Contains(view, "icmp") {
		t.Fatalf("ping result card did not show icmp fallback marker:\n%s", view)
	}
}

func TestIPQualityResultCardShowsFailClosedPortEvidence(t *testing.T) {
	report := checkup.CheckupReport{IPQuality: checkup.IPQualitySection{
		SectionState: checkup.SectionState{
			Enabled: true,
			Status:  "error",
			Message: "ip quality: port 25 probe inconclusive",
		},
		Result: &checkup.IPQualityResult{
			BasicInfo:   &netio.IPBasicInfo{IP: "203.0.113.10", CountryCode: "ZZ", ASN: 64500},
			RiskSummary: &netio.IPRiskSummary{Summary: "port25 error"},
			Port25: &netio.PortProbe{
				Port: 25, Status: netio.MailPortStatusError, Message: "resolver unavailable",
			},
		},
	}}

	view := ipQualityResultCard(report, 80)
	for _, want := range []string{"port 25 probe inconclusive", "port25 error", "Port 25", "resolver unavailable"} {
		if !strings.Contains(view, want) {
			t.Fatalf("IP Quality result card missing %q:\n%s", want, view)
		}
	}
}

func TestCheckupPagesFit80x24Terminal(t *testing.T) {
	const (
		width  = 80
		height = 24
	)
	info := sysinfo.SystemInfo{CPU: sysinfo.CPUInfo{
		Model:         "AMD EPYC-Milan Processor",
		PhysicalCores: 6,
		LogicalCores:  6,
	}}
	report := compactCheckupReportFixture()

	tests := []struct {
		name     string
		page     page
		setup    func(*Model)
		expected []string
	}{
		{
			name: "config",
			page: pageConfig,
			setup: func(m *Model) {
				m.config.advancedOpen = true
			},
			expected: []string{"Benchmark Configuration", "Hardware", "Media Unlock", "Advanced", "Start Benchmark"},
		},
		{
			name: "running",
			page: pageRunning,
			setup: func(m *Model) {
				m.runKind = "checkup"
				m.checkupSections = compactCheckupSectionsFixture()
			},
			expected: []string{"Running Checkup", "Hardware", "Reachability", "Mail Ports"},
		},
		{
			name:     "results",
			page:     pageCheckupResults,
			setup:    func(m *Model) { m.checkupReport = &report },
			expected: []string{"Checkup Report", "Hardware", "Network Info", "Media Unlock"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewModel("", "")
			m.page = tt.page
			m.width = width
			m.height = height
			m.sysInfo = info
			if tt.setup != nil {
				tt.setup(&m)
			}

			view := m.View()
			assertRenderBounds(t, view, width, height)
			for _, expected := range tt.expected {
				if !strings.Contains(view, expected) {
					t.Errorf("view does not contain %q", expected)
				}
			}
		})
	}
}

// TestCheckupPagesFit80x24TerminalZhCN re-runs the fixture pages under zh-CN:
// every label becomes double-width CJK, so the ≤80-cell bound is the gate
// that catches truncation and padding regressions.
func TestCheckupPagesFit80x24TerminalZhCN(t *testing.T) {
	if !i18n.SetLang("zh-CN") {
		t.Fatal("zh-CN not supported")
	}
	t.Cleanup(func() { i18n.SetLang("en") })

	const (
		width  = 80
		height = 24
	)
	info := sysinfo.SystemInfo{CPU: sysinfo.CPUInfo{
		Model:         "AMD EPYC-Milan Processor",
		PhysicalCores: 6,
		LogicalCores:  6,
	}}
	report := compactCheckupReportFixture()

	tests := []struct {
		name     string
		page     page
		setup    func(*Model)
		expected []string
	}{
		{
			name: "config",
			page: pageConfig,
			setup: func(m *Model) {
				m.config.advancedOpen = true
			},
			expected: []string{"评测配置", "硬件", "高级设置", "开始评测"},
		},
		{
			name: "running",
			page: pageRunning,
			setup: func(m *Model) {
				m.runKind = "checkup"
				m.checkupSections = compactCheckupSectionsFixture()
			},
			expected: []string{"综合测试运行中", "硬件"},
		},
		{
			name:     "results",
			page:     pageCheckupResults,
			setup:    func(m *Model) { m.checkupReport = &report },
			expected: []string{"综合测试报告", "网络信息"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewModel("", "")
			m.page = tt.page
			m.width = width
			m.height = height
			m.sysInfo = info
			if tt.setup != nil {
				tt.setup(&m)
			}

			view := m.View()
			assertRenderBounds(t, view, width, height)
			for _, expected := range tt.expected {
				if !strings.Contains(view, expected) {
					t.Errorf("view does not contain %q:\n%s", expected, view)
				}
			}
		})
	}
}

func TestCompactCheckupConfigKeepsRowNavigation(t *testing.T) {
	m := NewModel("", "")
	m.page = pageConfig
	m.width = 80
	m.height = 24

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	got := updated.(Model)
	view := got.View()
	assertRenderBounds(t, view, 80, 24)
	if row := got.config.currentRow(); row.kind != rowSection || row.index != 0 {
		t.Fatalf("down key did not move config to first section: row=%+v", row)
	}
	if !strings.Contains(view, "Hardware") {
		t.Fatalf("first section row should be visible:\n%s", view)
	}
}

func TestCompactCheckupConfigRowsFit80x24(t *testing.T) {
	m := NewModel("", "")
	m.page = pageConfig
	m.width = 80
	m.height = 24
	m.config.advancedOpen = true

	rows := m.config.visibleRows()
	for cursor := 0; cursor < len(rows); cursor++ {
		m.config.cursor = cursor
		view := m.View()
		assertRenderBounds(t, view, 80, 24)
		if !strings.Contains(view, "Start Benchmark") {
			t.Fatalf("start row should stay visible with cursor %d:\n%s", cursor, view)
		}
	}
}

func TestCompactCheckupRunningWorstCaseFits80x24(t *testing.T) {
	m := NewModel("", "")
	m.page = pageRunning
	m.runKind = "checkup"
	m.width = 80
	m.height = 24
	m.checkupSections = compactCheckupSectionsFixture()
	m.showLog = true
	m.confirm = true
	m.eventLog = []string{
		"12:00:00 start hardware with a deliberately long event detail",
		"12:00:01 done hardware with a deliberately long event detail",
		"12:00:02 start network_info with a deliberately long event detail",
	}

	view := m.View()
	assertRenderBounds(t, view, 80, 24)
	for _, expected := range []string{"Media Unlock", "Recent events", "Cancel checkup?"} {
		if !strings.Contains(view, expected) {
			t.Errorf("worst-case compact running view does not contain %q", expected)
		}
	}
}

func assertRenderBounds(t *testing.T, view string, width, height int) {
	t.Helper()
	if got := lipgloss.Height(view); got > height {
		t.Fatalf("render height = %d, want <= %d", got, height)
	}
	for lineNumber, line := range strings.Split(view, "\n") {
		if got := lipgloss.Width(line); got > width {
			t.Fatalf("line %d width = %d, want <= %d", lineNumber+1, got, width)
		}
	}
}

func compactCheckupSectionsFixture() []checkupSection {
	return newCheckupSections(checkup.SectionSelector{
		Hardware: true, NetworkInfo: true, Route: true, Ping: true, Speed: true,
		IPQuality: true, Reachability: true, Mail: true, Media: true,
	})
}

func compactCheckupReportFixture() checkup.CheckupReport {
	ok := checkup.SectionState{Enabled: true, Status: "ok", Message: "complete"}
	waiting := checkup.SectionState{Enabled: true, Status: "error", Message: "probe unavailable"}
	return checkup.CheckupReport{
		Status:       "failed",
		Message:      "8/9 sections ok; failed: speed",
		Hardware:     checkup.HardwareSection{SectionState: ok},
		NetworkInfo:  checkup.NetworkInfoSection{SectionState: ok},
		Route:        checkup.RouteSection{SectionState: ok},
		Ping:         checkup.PingSection{SectionState: ok},
		Speed:        checkup.SpeedSection{SectionState: waiting},
		IPQuality:    checkup.IPQualitySection{SectionState: ok},
		Reachability: checkup.ReachabilitySection{SectionState: ok},
		Mail:         checkup.MailSection{SectionState: ok},
		Media:        checkup.MediaSection{SectionState: ok},
	}
}
