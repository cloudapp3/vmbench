package report

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/cloudapp3/vmbench/bench"
	"github.com/cloudapp3/vmbench/sysinfo"
)

func TestBuildDocumentPreservesRawSamples(t *testing.T) {
	doc := BuildDocument("test", zeroSystemInfo(), RunConfig{Scope: "hardware"}, []bench.BenchResult{{
		Workload: "sample",
		Category: "CPU",
		Result: &bench.RunDetail{
			Iterations:     2,
			MedianTime:     20 * time.Millisecond,
			Samples:        []time.Duration{10 * time.Millisecond, 20 * time.Millisecond},
			BytesProcessed: 1024,
			OpsProcessed:   2,
		},
	}}, nil)
	if doc.SchemaVersion != currentSchemaVersion {
		t.Fatalf("schema version = %d, want %d", doc.SchemaVersion, currentSchemaVersion)
	}
	if doc.Config.Scope != "hardware" {
		t.Fatalf("config scope = %q, want hardware", doc.Config.Scope)
	}
	result := doc.Results.Workloads[0].Result
	if result.Iterations != 2 || len(result.SamplesMS) != 2 || result.SamplesMS[0] != 10 {
		t.Fatalf("raw sample fields = %+v", result)
	}
	if result.BytesProcessed != 1024 || result.OpsProcessed != 2 {
		t.Fatalf("processed fields = %+v", result)
	}
}

func TestBuildDocumentOmitsUnknownProcessedMetrics(t *testing.T) {
	doc := BuildDocument("test", zeroSystemInfo(), RunConfig{}, []bench.BenchResult{{
		Workload: "rate-only",
		Category: "CPU",
		Result: &bench.RunDetail{
			Iterations:     1,
			Throughput:     123,
			ThroughputUnit: "events/sec",
		},
	}}, nil)
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	encoded := string(data)
	for _, field := range []string{"bytes_processed", "ops_processed"} {
		if strings.Contains(encoded, field) {
			t.Fatalf("unknown processed metric emitted %q: %s", field, encoded)
		}
	}
}

func TestHasFailures(t *testing.T) {
	if !HasFailures(Document{}) {
		t.Fatal("empty document must fail")
	}
	ok := Document{Results: ResultsSection{Workloads: []WorkloadEntry{{Result: &ResultEntry{MedianMS: 1}}}}}
	if HasFailures(ok) {
		t.Fatal("successful document reported failure")
	}
	ok.Results.Workloads[0].Result.Error = "failed"
	if !HasFailures(ok) {
		t.Fatal("workload error was not reported")
	}
}

func zeroSystemInfo() sysinfo.SystemInfo {
	return sysinfo.SystemInfo{}
}
