package checkup

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	vmbench "github.com/cloudapp3/vmbench"
)

func TestStatusFromCounts(t *testing.T) {
	tests := []struct {
		name      string
		okCount   int
		failCount int
		want      string
	}{
		{name: "no results", want: "skipped"},
		{name: "all failed", failCount: 2, want: "error"},
		{name: "all succeeded", okCount: 2, want: "ok"},
		{name: "some failed", okCount: 2, failCount: 1, want: "partial"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := statusFromCounts(tt.okCount, tt.failCount); got != tt.want {
				t.Fatalf("statusFromCounts(%d, %d) = %q, want %q", tt.okCount, tt.failCount, got, tt.want)
			}
		})
	}
}

func TestCheckupReportHasFailures(t *testing.T) {
	tests := []struct {
		name    string
		status  string
		enabled bool
		want    bool
	}{
		{name: "ok", status: "ok", enabled: true},
		{name: "error", status: "error", enabled: true, want: true},
		{name: "failed", status: "failed", enabled: true, want: true},
		{name: "partial", status: "partial", enabled: true, want: true},
		{name: "normalized partial", status: " PARTIAL ", enabled: true, want: true},
		{name: "skipped enabled section", status: "skipped", enabled: true, want: true},
		{name: "empty enabled status", enabled: true, want: true},
		{name: "disabled partial", status: "partial"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := CheckupReport{
				Hardware: HardwareSection{SectionState: SectionState{Enabled: tt.enabled, Status: tt.status}},
			}
			if !tt.enabled {
				report.Mail.SectionState = SectionState{Enabled: true, Status: "ok"}
			}
			if got := report.HasFailures(); got != tt.want {
				t.Fatalf("HasFailures() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestCheckupReportHasFailuresWhenNoSectionsEnabled(t *testing.T) {
	if !(CheckupReport{}).HasFailures() {
		t.Fatal("HasFailures() = false, want true when no sections are enabled")
	}
}

func TestNewCheckupReportIncludesCompatibleEnvelope(t *testing.T) {
	oldVersion, oldCommit, oldBuildTime := vmbench.Version, vmbench.Commit, vmbench.BuildTime
	vmbench.Version, vmbench.Commit, vmbench.BuildTime = "v0.2.0-test", "abc123", "2026-07-13T00:00:00Z"
	t.Cleanup(func() {
		vmbench.Version, vmbench.Commit, vmbench.BuildTime = oldVersion, oldCommit, oldBuildTime
	})

	report := NewCheckupReport(Options{Sections: SectionSelector{Ping: true}})
	if report.SchemaVersion != 2 || report.ReportKind != "checkup" || report.Version != 1 {
		t.Fatalf("report envelope = schema %d kind %q legacy %d", report.SchemaVersion, report.ReportKind, report.Version)
	}
	if report.ReportID == "" || report.App.Version != "v0.2.0-test" || report.App.Commit != "abc123" || report.App.BuildTime == "" {
		t.Fatalf("report identity = id %q app %+v", report.ReportID, report.App)
	}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"schema_version":2`, `"report_kind":"checkup"`, `"version":1`, `"app":`, `"system":`} {
		if !strings.Contains(string(data), field) {
			t.Errorf("checkup JSON missing %s: %s", field, data)
		}
	}
}

func TestRunPopulatesEnvelopeForNetworkOnlyCheckup(t *testing.T) {
	report := Run(context.Background(), Options{
		Sections:       SectionSelector{Speed: true},
		SpeedProviders: []string{SpeedProviderIperf3},
		Timeout:        time.Second,
	})
	if report.StartedAt.IsZero() || report.FinishedAt.IsZero() || report.FinishedAt.Before(report.StartedAt) {
		t.Fatalf("checkup timestamps = %s to %s", report.StartedAt, report.FinishedAt)
	}
	if report.StartedTime == 0 || report.FinishedTime == 0 || report.DurationMS < 0 {
		t.Fatalf("legacy/duration timestamps = %d %d %d", report.StartedTime, report.FinishedTime, report.DurationMS)
	}
	if report.App.Version == "" || report.ReportID == "" {
		t.Fatalf("network-only checkup metadata = app %+v id %q", report.App, report.ReportID)
	}
	if report.System.CPU.Arch == "" && report.System.OS.GoVersion == "" {
		t.Fatalf("network-only checkup system metadata is empty: %+v", report.System)
	}
}
