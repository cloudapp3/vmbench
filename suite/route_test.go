package suite

import (
	"bytes"
	"strings"
	"testing"

	"github.com/cloudapp3/vmbench/bench/netio"
)

func TestSummarizeRouteResultsUsesDestinationStatus(t *testing.T) {
	reached := true
	notReached := false
	tests := []struct {
		name        string
		results     []netio.TraceProbeResult
		wantStatus  string
		wantMessage string
	}{
		{name: "none", wantStatus: "error", wantMessage: "no traceroute targets selected"},
		{
			name: "all destinations reached",
			results: []netio.TraceProbeResult{
				{Status: netio.TraceStatusOK, DestinationReached: &reached},
				{Status: netio.TraceStatusOK, DestinationReached: &reached},
			},
			wantStatus:  "ok",
			wantMessage: "2/2 destinations reached",
		},
		{
			name: "hops without destination are partial",
			results: []netio.TraceProbeResult{
				{Status: netio.TraceStatusPartial, DestinationReached: &notReached, Hops: []netio.Hop{{TTL: 1, IP: "192.0.2.1"}}},
				{Status: netio.TraceStatusError, DestinationReached: &notReached, Error: "command missing"},
			},
			wantStatus:  "partial",
			wantMessage: "0/2 destinations reached; 1 partial; 1 errors",
		},
		{
			name: "legacy error-free result remains readable",
			results: []netio.TraceProbeResult{
				{Hops: []netio.Hop{{TTL: 1, IP: "192.0.2.1"}}},
			},
			wantStatus:  "ok",
			wantMessage: "1/1 destinations reached",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, message := summarizeRouteResults(tt.results)
			if status != tt.wantStatus || message != tt.wantMessage {
				t.Fatalf("summarizeRouteResults() = %q, %q; want %q, %q", status, message, tt.wantStatus, tt.wantMessage)
			}
		})
	}
}

func TestWriteConsoleIncludesRouteReachabilityEvidence(t *testing.T) {
	notReached := false
	report := SuiteReport{Route: RouteSection{
		SectionState: SectionState{Enabled: true, Status: "partial", Message: "0/1 destinations reached; 1 partial"},
		Results: []RouteRun{{
			Target:             netio.TraceTarget{Name: "Route target", City: "Chengdu", Carrier: "CERNET"},
			ResolvedTarget:     "203.0.113.8",
			DestinationReached: &notReached,
			Status:             netio.TraceStatusPartial,
			ProbeProtocol:      "tcp",
			ProbeTool:          "traceroute",
			Hops:               []netio.Hop{{TTL: 1, IP: "192.0.2.1"}},
		}},
	}}
	var output bytes.Buffer
	if err := WriteConsole(&output, report); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"destinations reached", "Resolved", "203.0.113.8", "Reached", "no", "partial"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("console output missing %q:\n%s", want, output.String())
		}
	}
}
