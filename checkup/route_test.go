package checkup

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
	report := CheckupReport{Route: RouteSection{
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
	for _, want := range []string{"destinations reached", "Resolved", "203.0.113.8", "Unknown line", "partial (unreached)"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("console output missing %q:\n%s", want, output.String())
		}
	}
}

func TestRouteLineTextPrefersClassificationLabel(t *testing.T) {
	classification := &netio.RouteClassification{
		Code: "ct_163", Label: "电信163    [普通线路]", Confidence: "confirmed", Rank: 2,
	}
	if got := RouteLineText(netio.TraceProbeResult{Classification: classification}); got != "电信163 [普通线路]" {
		t.Fatalf("RouteLineText() = %q; want collapsed label", got)
	}
}

func TestRouteLineTextTranslatesObservedASNs(t *testing.T) {
	tests := []struct {
		name string
		asns []string
		want string
	}{
		{
			name: "AS4809 with AS4134 is the CN2 GT hand-off",
			asns: []string{"AS4134", "AS4809"},
			want: "电信CN2GT [优质线路] 电信163 [普通线路]",
		},
		{
			name: "AS4809 alone indicates the CN2 GIA backbone",
			asns: []string{"AS4809"},
			want: "电信CN2GIA [精品线路]",
		},
		{
			name: "plain 163 evidence stays visible",
			asns: []string{"AS4134"},
			want: "电信163 [普通线路]",
		},
		{
			name: "unknown ASNs produce no label",
			asns: []string{"AS64512"},
			want: "Unknown line",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RouteLineText(netio.TraceProbeResult{ObservedASNs: tt.asns}); got != tt.want {
				t.Fatalf("RouteLineText(%v) = %q; want %q", tt.asns, got, tt.want)
			}
		})
	}
}

func TestRouteLineTextIgnoresInconclusiveClassification(t *testing.T) {
	classification := &netio.RouteClassification{
		Code: "cu_unknown", Label: "线路证据不足", Confidence: "inconclusive",
	}
	result := netio.TraceProbeResult{Classification: classification, ObservedASNs: []string{"AS9929"}}
	if got := RouteLineText(result); got != "联通9929 [优质线路]" {
		t.Fatalf("RouteLineText() = %q; want observed ASN fallback", got)
	}
	if got := RouteLineText(netio.TraceProbeResult{Classification: classification}); got != "Unknown line" {
		t.Fatalf("RouteLineText() = %q; want unknown-line fallback", got)
	}
}

func TestRouteLineTextWithoutEvidenceIsLocalized(t *testing.T) {
	if got := RouteLineText(netio.TraceProbeResult{}); got == "" {
		t.Fatal("RouteLineText() = empty; want a non-empty unknown-line label")
	}
}

func TestRouteLineToneGradesEvidence(t *testing.T) {
	tests := []struct {
		name   string
		result netio.TraceProbeResult
		want   string
	}{
		{
			name: "premium classification is ok",
			result: netio.TraceProbeResult{Classification: &netio.RouteClassification{
				Code: "ct_cn2_gia", Label: "电信CN2GIA [精品线路]", Confidence: "confirmed", Rank: 5,
			}},
			want: "ok",
		},
		{
			name: "plain 163 is muted",
			result: netio.TraceProbeResult{Classification: &netio.RouteClassification{
				Code: "ct_163", Label: "电信163 [普通线路]", Confidence: "confirmed", Rank: 2,
			}},
			want: "muted",
		},
		{
			name: "inconclusive with quality ASN evidence is ok",
			result: netio.TraceProbeResult{
				Classification: &netio.RouteClassification{Code: "cu_unknown", Confidence: "inconclusive"},
				ObservedASNs:   []string{"AS9929"},
			},
			want: "ok",
		},
		{
			name:   "plain ASN evidence without classification is muted",
			result: netio.TraceProbeResult{ObservedASNs: []string{"AS4134"}},
			want:   "muted",
		},
		{
			name:   "no evidence is warn",
			result: netio.TraceProbeResult{},
			want:   "warn",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RouteLineTone(tt.result); got != tt.want {
				t.Fatalf("RouteLineTone() = %q; want %q", got, tt.want)
			}
		})
	}
}
