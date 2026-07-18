package report

import (
	"bytes"
	"strings"
	"testing"
)

func TestCompareTreatsMillisecondThroughputAsLowerIsBetter(t *testing.T) {
	docs := []Document{
		docWithResult("Ping", 10, "ms avg", ""),
		docWithResult("Ping", 20, "ms avg", ""),
	}
	var out bytes.Buffer
	if err := WriteCompare(&out, docs); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "▼-100.0%") {
		t.Fatalf("expected slower ping to be a regression:\n%s", out.String())
	}
}

func TestCompareRejectsMismatchedThroughputUnits(t *testing.T) {
	docs := []Document{
		docWithResult("Workload", 10, "MiB/s", ""),
		docWithResult("Workload", 20, "IOPS", ""),
	}
	var out bytes.Buffer
	if err := WriteCompare(&out, docs); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "incompatible units") {
		t.Fatalf("expected unit mismatch warning:\n%s", out.String())
	}
}

func TestCompareRejectsMissingVersusKnownThroughputUnit(t *testing.T) {
	docs := []Document{
		docWithResult("Workload", 10, "", ""),
		docWithResult("Workload", 20, "IOPS", ""),
	}
	var out bytes.Buffer
	if err := WriteCompare(&out, docs); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "incompatible units") {
		t.Fatalf("expected missing/known unit mismatch warning:\n%s", out.String())
	}
}

func TestCompareWarnsAboutMismatchedScopes(t *testing.T) {
	docs := []Document{
		{Config: RunConfig{Scope: "hardware"}},
		{Config: RunConfig{Scope: "network"}},
	}
	warnings := comparabilityWarnings(docs)
	if len(warnings) != 1 || !strings.Contains(warnings[0], "scope") {
		t.Fatalf("comparability warnings = %v, want scope warning", warnings)
	}
}

func TestCompareWarnsAboutMismatchedIperfTargets(t *testing.T) {
	docs := []Document{
		{Config: RunConfig{Scope: "network", IperfHosts: []string{"one.example:5201"}}},
		{Config: RunConfig{Scope: "network", IperfHosts: []string{"two.example:5201"}}},
	}
	warnings := comparabilityWarnings(docs)
	if len(warnings) != 1 || !strings.Contains(warnings[0], "iperf") {
		t.Fatalf("comparability warnings = %v, want iperf target warning", warnings)
	}
}

func docWithResult(name string, throughput float64, unit, resultErr string) Document {
	return Document{Results: ResultsSection{Workloads: []WorkloadEntry{{
		Name: name,
		Result: &ResultEntry{
			ThroughputPerSec: throughput,
			ThroughputUnit:   unit,
			Error:            resultErr,
		},
	}}}}
}
