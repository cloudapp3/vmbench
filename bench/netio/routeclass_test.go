package netio

import (
	"context"
	"testing"
)

func fixtureTraceResult(carrier, resolved string, hopIPs []string) TraceProbeResult {
	hops := make([]Hop, 0, len(hopIPs))
	for i, ip := range hopIPs {
		hops = append(hops, Hop{TTL: i + 1, IP: ip, RTTMs: float64(i+1) * 5})
	}
	return TraceProbeResult{
		Target:         TraceTarget{Name: "fixture", Carrier: carrier, City: "Guangzhou"},
		ResolvedTarget: resolved,
		Status:         TraceStatusOK,
		Hops:           hops,
	}
}

func TestClassifyTraceResultsTelecom163(t *testing.T) {
	results := []TraceProbeResult{
		fixtureTraceResult("CT", "202.97.1.1", []string{"10.0.0.1", "202.97.10.1", "202.97.20.1", "113.10.200.1"}),
	}
	ClassifyTraceResults(context.Background(), results)
	c := results[0].Classification
	if c == nil {
		t.Fatal("expected classification to be set")
	}
	if c.Code != "ct_163" {
		t.Errorf("classification code = %q, want ct_163 (label %q)", c.Code, c.Label)
	}
	if len(results[0].ObservedASNs) == 0 {
		t.Error("expected observed ASNs to be recorded")
	}
}

func TestClassifyTraceResultsTelecomCN2GIA(t *testing.T) {
	results := []TraceProbeResult{
		fixtureTraceResult("CT", "59.43.1.1", []string{"10.0.0.1", "59.43.10.1", "59.43.20.1", "59.43.30.1"}),
	}
	ClassifyTraceResults(context.Background(), results)
	c := results[0].Classification
	if c == nil {
		t.Fatal("expected classification to be set")
	}
	if c.Code != "ct_cn2_gia" {
		t.Errorf("classification code = %q, want ct_cn2_gia (label %q)", c.Code, c.Label)
	}
}

func TestClassifyTraceResultsUnicom4837(t *testing.T) {
	results := []TraceProbeResult{
		fixtureTraceResult("CU", "219.158.1.1", []string{"10.0.0.1", "219.158.10.1", "219.158.20.1"}),
	}
	ClassifyTraceResults(context.Background(), results)
	c := results[0].Classification
	if c == nil {
		t.Fatal("expected classification to be set")
	}
	if c.Code != "cu_4837" {
		t.Errorf("classification code = %q, want cu_4837 (label %q)", c.Code, c.Label)
	}
}

func TestClassifyTraceResultsSkipsEmptyAndUnresolvable(t *testing.T) {
	noHops := fixtureTraceResult("CT", "202.97.1.1", nil)
	notResolved := fixtureTraceResult("CT", "", []string{"202.97.10.1"})
	ClassifyTraceResults(context.Background(), []TraceProbeResult{noHops, notResolved})
	if noHops.Classification != nil {
		t.Error("expected no classification for result without hops")
	}
	if notResolved.Classification != nil {
		t.Error("expected no classification for result without resolved target")
	}
}

func TestClassifyTraceResultsNilContext(t *testing.T) {
	results := []TraceProbeResult{
		fixtureTraceResult("CM", "223.120.1.1", []string{"10.0.0.1", "223.120.10.1"}),
	}
	ClassifyTraceResults(nil, results)
	if results[0].Classification == nil {
		t.Fatal("expected classification with nil context (defaults to background)")
	}
}

func TestToBacktraceHopsDropsTimeouts(t *testing.T) {
	hops := []Hop{
		{TTL: 1, IP: "1.2.3.4", RTTMs: 3.5},
		{TTL: 2, Timeout: true},
		{TTL: 3, IP: "not-an-ip"},
		{TTL: 4, IP: "5.6.7.8"},
	}
	converted := toBacktraceHops(hops)
	if len(converted) != 2 {
		t.Fatalf("converted %d hops, want 2", len(converted))
	}
	if converted[0].Distance != 1 || len(converted[0].Nodes) != 1 || converted[0].Nodes[0].IP.String() != "1.2.3.4" {
		t.Errorf("unexpected first converted hop: %+v", converted[0])
	}
	if converted[1].Distance != 4 {
		t.Errorf("unexpected second converted hop distance: %d", converted[1].Distance)
	}
}
