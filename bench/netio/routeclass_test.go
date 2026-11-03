package netio

import (
	"context"
	"reflect"
	"strings"
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

func classifyFixture(t *testing.T, carrier, resolved string, hopIPs []string) TraceProbeResult {
	t.Helper()
	results := []TraceProbeResult{fixtureTraceResult(carrier, resolved, hopIPs)}
	ClassifyTraceResults(context.Background(), results)
	if results[0].Classification == nil {
		t.Fatal("expected classification to be set")
	}
	return results[0]
}

func TestClassifyTraceResultsTelecom163(t *testing.T) {
	result := classifyFixture(t, "CT", "202.97.1.1", []string{"10.0.0.1", "202.97.10.1", "202.97.20.1", "113.10.200.1"})
	if result.Classification.Code != "ct_163" {
		t.Errorf("classification code = %q, want ct_163 (label %q)", result.Classification.Code, result.Classification.Label)
	}
	if len(result.ObservedASNs) == 0 {
		t.Error("expected observed ASNs to be recorded")
	}
}

func TestClassifyTraceResultsTelecomCN2GIA(t *testing.T) {
	// A true GIA return: CTGNet handoff, CN2 carries ahead of 163 delivery
	// hops, destination sits on the 163 network.
	result := classifyFixture(t, "CT", "202.97.1.1", []string{"10.0.0.1", "203.22.10.1", "59.43.10.1", "59.43.20.1", "202.97.10.1"})
	if result.Classification.Code != "ct_cn2_gia" {
		t.Errorf("classification code = %q, want ct_cn2_gia (label %q)", result.Classification.Code, result.Classification.Label)
	}
	if result.Classification.Rank != routeRankPremium {
		t.Errorf("rank = %d, want %d", result.Classification.Rank, routeRankPremium)
	}
}

func TestClassifyTraceResultsTelecomCN2DestinationDelivery(t *testing.T) {
	// Regression (2026-11 sin report): destination hosted on the CN2 network
	// makes every trace end on trailing 59.43 hops regardless of how the
	// machine's traffic enters China. That is delivery, not carriage.
	result := classifyFixture(t, "CT", "59.43.1.1", []string{"10.0.0.1", "129.250.6.70", "59.43.38.193", "59.43.159.17"})
	if result.Classification.Code != "ct_cn2_mixed" {
		t.Errorf("classification code = %q, want ct_cn2_mixed (label %q)", result.Classification.Code, result.Classification.Label)
	}
	if result.Classification.Confidence != routeConfidenceMixed {
		t.Errorf("confidence = %q, want mixed", result.Classification.Confidence)
	}
}

func TestClassifyTraceResultsTelecomCTGNetIsOrdinary(t *testing.T) {
	// Regression (2026-11 sin report): AS23764 presence alone used to claim a
	// premium CTGNET line; an international handoff without domestic CN2/163
	// evidence is ordinary CT international transit.
	result := classifyFixture(t, "CT", "59.43.9.9", []string{"10.0.0.1", "129.250.6.70", "203.22.177.85", "69.194.169.94"})
	if result.Classification.Code != "ct_ctgnet" {
		t.Fatalf("classification code = %q, want ct_ctgnet", result.Classification.Code)
	}
	if result.Classification.Rank != routeRankOrdinary {
		t.Errorf("rank = %d, want %d (ordinary)", result.Classification.Rank, routeRankOrdinary)
	}
	if !strings.Contains(result.Classification.Label, "普通线路") {
		t.Errorf("label %q should read as an ordinary line", result.Classification.Label)
	}
}

func TestClassifyTraceResultsUnicom4837(t *testing.T) {
	result := classifyFixture(t, "CU", "219.158.1.1", []string{"10.0.0.1", "219.158.10.1", "219.158.20.1"})
	if result.Classification.Code != "cu_4837" {
		t.Errorf("classification code = %q, want cu_4837 (label %q)", result.Classification.Code, result.Classification.Label)
	}
}

func TestClassifyTraceResultsUnicom9929(t *testing.T) {
	// True 9929 return: CUII carries ahead of 4837 delivery hops.
	result := classifyFixture(t, "CU", "219.158.1.1", []string{"10.0.0.1", "218.105.10.1", "218.105.20.1", "219.158.10.1"})
	if result.Classification.Code != "cu_9929" {
		t.Errorf("classification code = %q, want cu_9929 (label %q)", result.Classification.Code, result.Classification.Label)
	}
}

func TestClassifyTraceResultsUnicom9929DestinationDelivery(t *testing.T) {
	// Regression (2026-11 sin report): a single 9929 hop delivered into an
	// A-network node is not 9929 carriage.
	result := classifyFixture(t, "CU", "218.105.1.1", []string{"10.0.0.1", "27.111.230.201", "218.105.2.165", "58.248.32.2"})
	if result.Classification.Code != "cu_9929_mixed" {
		t.Errorf("classification code = %q, want cu_9929_mixed (label %q)", result.Classification.Code, result.Classification.Label)
	}
}

func TestClassifyTraceResultsMobileCMIOrdinary(t *testing.T) {
	// Regression (2026-11 sin report): hops read AS58453 CMI then AS9808
	// CMNET, but the classification used to claim CMIN2 premium because a
	// second ASN database inside backtrace labeled the same prefixes AS58807.
	result := classifyFixture(t, "CM", "223.120.200.1", []string{"10.0.0.1", "223.120.130.21", "223.120.140.5", "221.183.92.153", "221.183.89.237"})
	if result.Classification.Code != "cm_cmi" {
		t.Errorf("classification code = %q, want cm_cmi (label %q)", result.Classification.Code, result.Classification.Label)
	}
	if result.Classification.Rank != routeRankOrdinary {
		t.Errorf("rank = %d, want %d (ordinary)", result.Classification.Rank, routeRankOrdinary)
	}
	// Observed ASNs must agree with the hop annotations.
	if want := []string{"AS58453", "AS9808"}; !reflect.DeepEqual(result.ObservedASNs, want) {
		t.Errorf("observed ASNs = %v, want %v", result.ObservedASNs, want)
	}
}

func TestClassifyTraceResultsMobileCMIN2(t *testing.T) {
	// True CMIN2 return: AS58807 space carried ahead of CMNET delivery hops.
	result := classifyFixture(t, "CM", "221.183.1.1", []string{"10.0.0.1", "223.118.34.1", "223.118.34.2", "221.183.10.1"})
	if result.Classification.Code != "cm_cmin2" {
		t.Errorf("classification code = %q, want cm_cmin2 (label %q)", result.Classification.Code, result.Classification.Label)
	}
}

func TestClassifyTraceResultsMobileCMIN2DestinationDelivery(t *testing.T) {
	// Destination hosted on CMIN2 space: trailing AS58807 hops are delivery.
	result := classifyFixture(t, "CM", "223.118.35.1", []string{"10.0.0.1", "223.118.34.1", "223.118.34.2"})
	if result.Classification.Code != "cm_cmin2_mixed" {
		t.Errorf("classification code = %q, want cm_cmin2_mixed (label %q)", result.Classification.Code, result.Classification.Label)
	}
}

func TestClassifyTraceResultsUnknownCarrier(t *testing.T) {
	result := classifyFixture(t, "CERNET", "202.97.1.1", []string{"10.0.0.1", "202.97.10.1"})
	if result.Classification.Confidence != routeConfidenceInconclusive {
		t.Errorf("confidence = %q, want inconclusive", result.Classification.Confidence)
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
