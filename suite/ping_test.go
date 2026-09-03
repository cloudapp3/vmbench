package suite

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/cloudapp3/vmbench/bench/netio"
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

func TestWriteConsoleIncludesPingConnectionState(t *testing.T) {
	report := SuiteReport{Ping: PingSection{
		SectionState: SectionState{Enabled: true, Status: "ok"},
		Results: []PingResult{{
			Name:            "Closed port",
			Status:          "ok",
			ConnectionState: netio.PingConnectionStateRefused,
			Sent:            10,
			Received:        10,
		}},
	}}
	var output bytes.Buffer
	if err := WriteConsole(&output, report); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Connection", "Closed port", "refused"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("console output missing %q:\n%s", want, output.String())
		}
	}
}
