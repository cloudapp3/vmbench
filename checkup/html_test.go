package checkup

import (
	"bytes"
	"strings"
	"testing"

	vmbench "github.com/cloudapp3/vmbench"
	"github.com/cloudapp3/vmbench/bench/netio"
)

func TestWriteHTMLHandlesMissingOptionalResults(t *testing.T) {
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
			name: "speed group without summary",
			report: CheckupReport{
				Speed: SpeedSection{
					SectionState: SectionState{Enabled: true, Status: "partial"},
					Result: &SpeedResult{
						Providers: []SpeedProviderResult{{ID: "failed", Provider: "test", Status: "error"}},
						Groups:    []SpeedProviderGroup{{ID: "test", Provider: "test", Status: "error"}},
					},
				},
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
			var output bytes.Buffer
			if err := WriteHTML(&output, tt.report); err != nil {
				t.Fatalf("WriteHTML() error = %v", err)
			}
			if !strings.Contains(output.String(), "<!doctype html>") {
				t.Fatalf("WriteHTML() output did not contain an HTML document")
			}
		})
	}
}

func TestWriteHTMLIncludesDetailedCheckupEvidence(t *testing.T) {
	report := NewCheckupReport(Options{Sections: SectionSelector{
		Hardware: true, NetworkInfo: true, Route: true, Ping: true, Reachability: true,
	}})
	report.Hardware.Report = &vmbench.Report{
		Results: vmbench.ResultsSection{Workloads: []vmbench.WorkloadEntry{{
			Name:     "CPU Single-Core",
			Category: "CPU",
			Result: &vmbench.ResultEntry{
				Iterations:       3,
				MedianMS:         12.5,
				ThroughputPerSec: 900,
				ThroughputUnit:   "events/s",
				Detail:           "measured detail",
			},
		}}},
	}
	report.NetworkInfo.Result = &NetworkIdentityResult{
		PublicIPv4: &PublicIPIdentity{IP: "203.0.113.10", IPVersion: "v4", ASN: 64500, Org: "Example ISP"},
		NAT: []NATHeuristic{{
			IPVersion: "v4", Status: "translated", Method: "test", PublicIP: "203.0.113.10", LocalIP: "10.0.0.2", Reason: "addresses differ",
		}},
		Providers: []NetworkIdentityProviderResult{{ID: "identity-provider", Kind: "metadata", Status: "ok"}},
	}
	notReached := false
	report.Route.Results = []RouteRun{{
		Target:             netio.TraceTarget{Name: "Route target", City: "Chengdu", Carrier: "CERNET", AS: 4538, Endpoint: "route.example"},
		ResolvedTarget:     "203.0.113.8",
		DestinationReached: &notReached,
		Status:             netio.TraceStatusPartial,
		Hops:               []netio.Hop{{TTL: 1, IP: "10.0.0.1", RTTMs: 1.25}},
	}}
	report.Ping.Results = []PingResult{{
		ID:              "closed-port",
		Name:            "Closed port",
		Target:          "203.0.113.9",
		Status:          "ok",
		ConnectionState: netio.PingConnectionStateRefused,
		Message:         "target responded with TCP RST",
		Sent:            10,
		Received:        10,
	}}
	report.Reachability.Results = []ReachabilityResult{{
		ID: "telegram_dc1", Category: "telegram", Protocol: "tcp", Endpoint: "149.154.175.53:443", Status: "reachable", LatencyMs: 20,
	}}

	var output bytes.Buffer
	if err := WriteHTML(&output, report); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"CPU Single-Core",
		"900.00 events/s",
		"203.0.113.10",
		"addresses differ",
		"Route target",
		"10.0.0.1",
		"203.0.113.8",
		"destination reached no",
		"partial",
		"Closed port",
		"refused",
		"TCP RST",
		"telegram_dc1",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("HTML did not contain %q", want)
		}
	}
}

func TestWriteHTMLIncludesRouteLineSummary(t *testing.T) {
	reached := true
	report := CheckupReport{Route: RouteSection{
		SectionState: SectionState{Enabled: true, Status: "ok"},
		Results: []RouteRun{{
			Target:             netio.TraceTarget{Name: "Guangzhou CT", City: "Guangzhou", Carrier: "CT", AS: 4134},
			ResolvedTarget:     "202.96.209.133",
			DestinationReached: &reached,
			Status:             netio.TraceStatusOK,
			Classification: &netio.RouteClassification{
				Code: "ct_cn2_gia", Label: "电信CN2GIA [精品线路]", Confidence: "confirmed", Rank: 5,
			},
			Hops: []netio.Hop{{TTL: 2, IP: "59.43.0.1", ASN: "AS4809"}},
		}},
	}}
	var output bytes.Buffer
	if err := WriteHTML(&output, report); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Guangzhou CT",
		"202.96.209.133",
		"电信CN2GIA [精品线路]",
		"confirmed",
		"badge ok",
		"AS4809",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("HTML route line summary missing %q", want)
		}
	}
}
