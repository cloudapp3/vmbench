package checkup

import (
	"bytes"
	"strings"
	"testing"
	"time"

	vmbench "github.com/cloudapp3/vmbench"
	"github.com/cloudapp3/vmbench/bench/netio"
)

func markdownCheckupReport() CheckupReport {
	report := NewCheckupReport(Options{Sections: SectionSelector{
		Hardware: true, Route: true, Ping: true, Mail: true, Media: true,
	}})
	report.Status = "ok"
	report.App = AppInfo{Version: "v0.14.0-test", Commit: "abc1234"}
	report.StartedAt = time.Date(2026, 9, 11, 7, 0, 0, 0, time.UTC)
	report.DurationMS = (3*60 + 10) * 1000
	report.Config.CatalogSource = "embedded"
	report.Config.CatalogRevision = "rev-42"
	report.Hardware.Report = &vmbench.Report{
		Version: "v0.14.0-test",
		Results: vmbench.ResultsSection{Workloads: []vmbench.WorkloadEntry{{
			Name:     "CPU Single-Core",
			Category: "CPU",
			Result: &vmbench.ResultEntry{
				MedianMS:         12.5,
				ThroughputPerSec: 900,
				ThroughputUnit:   "events/s",
				Detail:           "measured detail",
			},
		}}},
	}
	reached := true
	report.Route.Results = []RouteRun{{
		Target:             netio.TraceTarget{Name: "Guangzhou CT", City: "Guangzhou", Carrier: "CT", AS: 4134},
		ResolvedTarget:     "202.96.209.133",
		DestinationReached: &reached,
		Status:             netio.TraceStatusOK,
		Classification: &netio.RouteClassification{
			Code: "ct_cn2_gia", Label: "电信CN2GIA [精品线路]", Confidence: "confirmed", Rank: 5,
		},
	}}
	report.Ping.Results = []PingResult{{
		Name: "Beijing CT", City: "Beijing", Carrier: "CT", Status: "ok",
		AvgLatencyMs: 12.3, JitterMs: 1.2, PacketLoss: 0,
		ConnectionState: netio.PingConnectionStateOpen,
	}}
	report.Mail.Results = []PortProbe{{
		Title: "25", Port: 25, Status: "open", LatencyMs: 20.5, Method: "tcp",
	}}
	report.Media.Result = &MediaResult{
		Set: "globe",
		Summary: MediaSummary{
			Available: 4, Restricted: 1, Blocked: 2, Unknown: 1,
		},
		Items: []MediaServiceResult{
			{ID: "netflix", Title: "Netflix", Status: "available", Region: "na", RawStatus: "Yes"},
			{ID: "hbo", Title: "HBO Max", Status: "available", Region: "na", RawStatus: "Restricted"},
			{ID: "disney", Title: "Disney+", Status: "blocked", Region: "na", RawStatus: "No", Message: "not available"},
			{ID: "abema", Title: "AbemaTV", Status: "available", Region: "jp", RawStatus: "Yes"},
			{ID: "tver", Title: "TVer", Status: "unknown", Region: "jp", RawStatus: "Timeout", Message: "probe timeout"},
			{ID: "openai", Title: "OpenAI", Status: "blocked", Region: "ai", RawStatus: "Banned"},
			{ID: "gemini", Title: "Gemini", Status: "available", Region: "ai", RawStatus: "Yes"},
		},
	}
	return report
}

func TestWriteMarkdownRendersSectionsWithFences(t *testing.T) {
	var out bytes.Buffer
	report := markdownCheckupReport()
	if err := WriteMarkdown(&out, report); err != nil {
		t.Fatal(err)
	}
	markdown := out.String()
	for _, want := range []string{
		"# VMBench v0.14.0-test — Checkup Report",
		"Status: ok · vmbench v0.14.0-test (abc1234) · 2026-09-11T07:00:00Z · duration: 3m10s · catalog embedded@rev-42",
		"CPU Single-Core",
		"Guangzhou CT",
		"Beijing CT",
	} {
		if !strings.Contains(markdown, want) {
			t.Fatalf("markdown missing %q:\n%s", want, markdown)
		}
	}
	if got := strings.Count(markdown, "```"); got%2 != 0 {
		t.Fatalf("expected balanced fenced blocks, got %d fence markers:\n%s", got, markdown)
	}
}

func TestWriteMarkdownFoldsMediaToRegionsAndExceptions(t *testing.T) {
	var out bytes.Buffer
	if err := WriteMarkdown(&out, markdownCheckupReport()); err != nil {
		t.Fatal(err)
	}
	markdown := out.String()
	for _, want := range []string{
		"total: available 4 · restricted 1 · blocked 2 · unknown 1",
		"na: 2/3 available",
		"jp: 1/2 available",
		"ai: 1/2 available",
		"exceptions:",
		"HBO Max [na] restricted",
		"Disney+ [na] blocked not available",
		"TVer [jp] unknown probe timeout",
		"OpenAI [ai] blocked",
	} {
		if !strings.Contains(markdown, want) {
			t.Fatalf("folded media missing %q:\n%s", want, markdown)
		}
	}
	for _, banned := range []string{"Netflix [", "AbemaTV [", "Gemini ["} {
		if strings.Contains(markdown, banned) {
			t.Fatalf("folded media must not list plain-available services, found %q:\n%s", banned, markdown)
		}
	}
}

func TestWriteMarkdownHandlesMissingOptionalResults(t *testing.T) {
	tests := []struct {
		name   string
		report CheckupReport
	}{
		{
			name:   "all optional sections disabled",
			report: CheckupReport{},
		},
		{
			name: "hardware only",
			report: NewCheckupReport(Options{
				Sections: SectionSelector{Hardware: true},
			}),
		},
		{
			name: "speed failed without result",
			report: CheckupReport{
				Speed: SpeedSection{SectionState: SectionState{Enabled: true, Status: "error"}},
			},
		},
		{
			name: "ip quality failed without result",
			report: CheckupReport{
				IPQuality: IPQualitySection{SectionState: SectionState{Enabled: true, Status: "error"}},
			},
		},
		{
			name: "media failed without result",
			report: CheckupReport{
				Media: MediaSection{SectionState: SectionState{Enabled: true, Status: "error"}},
			},
		},
		{
			name: "empty optional results",
			report: CheckupReport{
				Speed:     SpeedSection{Result: &SpeedResult{}},
				IPQuality: IPQualitySection{Result: &IPQualityResult{}},
				Media:     MediaSection{Result: &MediaResult{}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			if err := WriteMarkdown(&out, tt.report); err != nil {
				t.Fatalf("WriteMarkdown() error = %v", err)
			}
			markdown := out.String()
			if !strings.Contains(markdown, "# VMBench") {
				t.Fatalf("WriteMarkdown() output did not contain the title heading")
			}
			if got := strings.Count(markdown, "```"); got%2 != 0 {
				t.Fatalf("unbalanced fenced blocks (%d markers):\n%s", got, markdown)
			}
		})
	}
}

func TestWriteMarkdownSkipsDisabledSections(t *testing.T) {
	var out bytes.Buffer
	report := markdownCheckupReport()
	report.Media.Enabled = false
	if err := WriteMarkdown(&out, report); err != nil {
		t.Fatal(err)
	}
	markdown := out.String()
	for _, want := range []string{"## Route", "## Ping", "## Mail", "## Hardware"} {
		if !strings.Contains(markdown, want) {
			t.Fatalf("markdown missing enabled section %q:\n%s", want, markdown)
		}
	}
	if strings.Contains(markdown, "## Media") || strings.Contains(markdown, "total: available") {
		t.Fatalf("markdown must skip disabled media section:\n%s", markdown)
	}
}
