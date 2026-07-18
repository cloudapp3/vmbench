package suite

import (
	"context"
	"testing"
	"time"
)

func TestRunPingSectionKeepsResultsWhenAllTargetsFail(t *testing.T) {
	opts := PrepareOptions(Options{
		Sections:  SectionSelector{Ping: true},
		IPVersion: "v4",
		Timeout:   time.Second,
	})
	report := NewSuiteReport(opts)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	runPingSection(ctx, opts, &report)

	if report.Ping.Status != "error" {
		t.Fatalf("Ping.Status = %q, want error", report.Ping.Status)
	}
	if len(report.Ping.Results) == 0 {
		t.Fatal("Ping.Results is empty, want per-target failures retained")
	}
	for i, result := range report.Ping.Results {
		if result.Status != "error" || result.Message == "" {
			t.Fatalf("Ping.Results[%d] = %+v, want structured error result", i, result)
		}
	}
}
