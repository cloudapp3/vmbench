package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/cloudapp3/vmbench/suite"
	"github.com/cloudapp3/vmbench/sysinfo"
)

func TestSuitePagesFitCompactTerminalWidth(t *testing.T) {
	const width = 80
	info := sysinfo.SystemInfo{CPU: sysinfo.CPUInfo{
		Model:         "AMD EPYC 9754 128-Core Processor",
		PhysicalCores: 128,
		LogicalCores:  256,
	}}
	report := suite.SuiteReport{
		Status:  "failed",
		Message: "0/1 sections ok; failed: speed",
		Speed: suite.SpeedSection{
			SectionState: suite.SectionState{Enabled: true, Status: "error"},
			Result: &suite.SpeedResult{Groups: []suite.SpeedProviderGroup{{
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
		{name: "config", page: pageSuiteConfig},
		{name: "running", page: pageSuiteRunning, setup: func(m *Model) {
			m.suiteSections = []suiteSection{
				{id: suite.SectionHardware, label: "Hardware", status: "done", message: "ok"},
				{id: suite.SectionNetworkInfo, label: "Network Info", status: "done", message: "ok"},
				{id: suite.SectionRoute, label: "Route", status: "done", message: "ok"},
				{id: suite.SectionPing, label: "Ping", status: "running"},
				{id: suite.SectionSpeed, label: "Speed", status: "waiting"},
				{id: suite.SectionIPQuality, label: "IP Quality", status: "waiting"},
				{id: suite.SectionReachability, label: "Reachability", status: "waiting"},
				{id: suite.SectionMail, label: "Mail Ports", status: "skip"},
			}
		}},
		{name: "results", page: pageSuiteResults, setup: func(m *Model) { m.suiteReport = &report }},
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

func TestSuitePagesFit80x24Terminal(t *testing.T) {
	const (
		width  = 80
		height = 24
	)
	info := sysinfo.SystemInfo{CPU: sysinfo.CPUInfo{
		Model:         "AMD EPYC-Milan Processor",
		PhysicalCores: 6,
		LogicalCores:  6,
	}}
	report := compactSuiteReportFixture()

	tests := []struct {
		name     string
		page     page
		setup    func(*Model)
		expected []string
	}{
		{
			name: "config",
			page: pageSuiteConfig,
			setup: func(m *Model) {
				m.suiteConfig.field = fieldAdvanced
			},
			expected: []string{"Suite Configuration", "Network Provenance", "Start Suite"},
		},
		{
			name: "running",
			page: pageSuiteRunning,
			setup: func(m *Model) {
				m.suiteSections = compactSuiteSectionsFixture()
			},
			expected: []string{"Running Suite", "Hardware", "Reachability", "Mail Ports"},
		},
		{
			name:     "results",
			page:     pageSuiteResults,
			setup:    func(m *Model) { m.suiteReport = &report },
			expected: []string{"Suite Report", "Hardware", "Network Info", "Media Unlock"},
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

func TestCompactSuiteConfigKeepsFieldNavigation(t *testing.T) {
	m := NewModel("", "")
	m.page = pageSuiteConfig
	m.width = 80
	m.height = 24

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	got := updated.(Model)
	view := got.View()
	assertRenderBounds(t, view, 80, 24)
	if got.suiteConfig.field != fieldSections || !strings.Contains(view, "Sections") {
		t.Fatalf("down key did not move compact config to Sections: field=%d", got.suiteConfig.field)
	}
}

func TestCompactSuiteConfigFieldsFit80x24(t *testing.T) {
	fields := []struct {
		field    suiteConfigField
		expected string
	}{
		{field: fieldPreset, expected: "Preset"},
		{field: fieldSections, expected: "Sections"},
		{field: fieldRuntime, expected: "Runtime"},
		{field: fieldHardwareTools, expected: "Hardware Tools"},
		{field: fieldSpeedProviders, expected: "Speed Providers"},
		{field: fieldRoutePresets, expected: "China Route Presets"},
		{field: fieldAdvanced, expected: "Network Provenance"},
		{field: fieldStart, expected: "Ready"},
	}
	for _, tt := range fields {
		t.Run(tt.expected, func(t *testing.T) {
			m := NewModel("", "")
			m.page = pageSuiteConfig
			m.width = 80
			m.height = 24
			m.suiteConfig.field = tt.field

			view := m.View()
			assertRenderBounds(t, view, 80, 24)
			if !strings.Contains(view, tt.expected) {
				t.Fatalf("compact config does not contain focused field %q", tt.expected)
			}
		})
	}
}

func TestCompactSuiteRunningWorstCaseFits80x24(t *testing.T) {
	m := NewModel("", "")
	m.page = pageSuiteRunning
	m.width = 80
	m.height = 24
	m.suiteSections = compactSuiteSectionsFixture()
	m.showLog = true
	m.confirm = true
	m.eventLog = []string{
		"12:00:00 start hardware with a deliberately long event detail",
		"12:00:01 done hardware with a deliberately long event detail",
		"12:00:02 start network_info with a deliberately long event detail",
	}

	view := m.View()
	assertRenderBounds(t, view, 80, 24)
	for _, expected := range []string{"Media Unlock", "Recent events", "Cancel suite?"} {
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

func compactSuiteSectionsFixture() []suiteSection {
	return []suiteSection{
		{id: suite.SectionHardware, label: "Hardware", status: "done", message: "ok"},
		{id: suite.SectionNetworkInfo, label: "Network Info", status: "done", message: "ok"},
		{id: suite.SectionRoute, label: "Route", status: "done", message: "ok"},
		{id: suite.SectionPing, label: "Ping", status: "running"},
		{id: suite.SectionSpeed, label: "Speed", status: "waiting"},
		{id: suite.SectionIPQuality, label: "IP Quality", status: "waiting"},
		{id: suite.SectionReachability, label: "Reachability", status: "waiting"},
		{id: suite.SectionMail, label: "Mail Ports", status: "skip"},
		{id: suite.SectionMedia, label: "Media Unlock", status: "waiting"},
	}
}

func compactSuiteReportFixture() suite.SuiteReport {
	ok := suite.SectionState{Enabled: true, Status: "ok", Message: "complete"}
	waiting := suite.SectionState{Enabled: true, Status: "error", Message: "probe unavailable"}
	return suite.SuiteReport{
		Status:       "failed",
		Message:      "8/9 sections ok; failed: speed",
		Hardware:     suite.HardwareSection{SectionState: ok},
		NetworkInfo:  suite.NetworkInfoSection{SectionState: ok},
		Route:        suite.RouteSection{SectionState: ok},
		Ping:         suite.PingSection{SectionState: ok},
		Speed:        suite.SpeedSection{SectionState: waiting},
		IPQuality:    suite.IPQualitySection{SectionState: ok},
		Reachability: suite.ReachabilitySection{SectionState: ok},
		Mail:         suite.MailSection{SectionState: ok},
		Media:        suite.MediaSection{SectionState: ok},
	}
}
