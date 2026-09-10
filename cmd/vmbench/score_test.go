package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func scoreTestReport(t *testing.T) string {
	t.Helper()
	doc := map[string]any{
		"schema_version": 2,
		"version":        "test",
		"timestamp":      "2026-09-09T00:00:00Z",
		"system":         map[string]any{"os": map[string]any{"name": "Ubuntu 24.04", "kernel": "6.8.0"}},
		"config": map[string]any{
			"iterations":     3,
			"hardware_tools": []string{"sysbench", "fio"},
		},
		"results": map[string]any{"workloads": []map[string]any{
			{
				"name": "CPU Single-Core (sysbench)",
				"result": map[string]any{
					"iterations": 3, "median_ms": 12.0,
					"throughput_per_sec": 3800.0, "throughput_unit": "events/sec",
					"samples_ms": []float64{11.5, 12.0, 12.5},
				},
			},
			{
				"name": "Disk 4K Random Read Q1 (fio)",
				"result": map[string]any{
					"iterations": 3, "median_ms": 0.2,
					"throughput_per_sec": 8000.0, "throughput_unit": "IOPS",
					"latency_p99_ns": 250000.0,
					"samples_ms":     []float64{0.2, 0.2, 0.2},
				},
			},
		}},
	}
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "report.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestUsageRowsIncludeScore(t *testing.T) {
	joined := strings.Join(usageRows(), "\n")
	if !strings.Contains(joined, "vmbench score") {
		t.Fatalf("usage rows lack score command:\n%s", joined)
	}
}

func TestRunScoreRequiresExactlyOneReport(t *testing.T) {
	if code := run([]string{"score"}); code != 2 {
		t.Fatalf("score without args exit = %d, want 2", code)
	}
	if code := run([]string{"score", "a.json", "b.json"}); code != 2 {
		t.Fatalf("score with two args exit = %d, want 2", code)
	}
}

func TestRunScoreMissingFileExitsOne(t *testing.T) {
	if code := run([]string{"score", filepath.Join(t.TempDir(), "absent.json")}); code != 1 {
		t.Fatalf("score with missing file exit = %d, want 1", code)
	}
}

func TestRunScoreConsoleAndJSON(t *testing.T) {
	path := scoreTestReport(t)

	output, code := captureStdout(t, func() int {
		return run([]string{"score", path})
	})
	if code != 0 {
		t.Fatalf("score exit = %d", code)
	}
	for _, want := range []string{"Baseline: 2026-09.1", "cpu", "disk", "web", "build", "proxy", "storage"} {
		if !strings.Contains(output, want) {
			t.Fatalf("console output lacks %q:\n%s", want, output)
		}
	}

	output, code = captureStdout(t, func() int {
		return run([]string{"score", "--json", path})
	})
	if code != 0 {
		t.Fatalf("score --json exit = %d", code)
	}
	var assessment struct {
		SchemaVersion int    `json:"schema_version"`
		ReportKind    string `json:"report_kind"`
		Baseline      struct {
			Revision string `json:"revision"`
		} `json:"baseline"`
		Composite *struct {
			Index float64 `json:"index"`
		} `json:"composite"`
	}
	if err := json.Unmarshal([]byte(output), &assessment); err != nil {
		t.Fatalf("assessment JSON invalid: %v\n%s", err, output)
	}
	if assessment.ReportKind != "assessment" || assessment.SchemaVersion != 1 {
		t.Fatalf("unexpected assessment header: %+v", assessment)
	}
	if assessment.Baseline.Revision != "2026-09.1" {
		t.Fatalf("baseline revision = %q", assessment.Baseline.Revision)
	}

	// Sparse report: cpu + disk present, memory/stability empty -> composite withheld.
	_, code = captureStdout(t, func() int {
		return run([]string{"score", "--json", path})
	})
	if code != 0 {
		t.Fatalf("second score exit = %d", code)
	}
}

func TestRunScoreRevisionPinMismatchExitsOne(t *testing.T) {
	path := scoreTestReport(t)
	if code := run([]string{"score", "--baseline-rev", "1999.9", path}); code != 1 {
		t.Fatalf("revision pin mismatch exit = %d, want 1", code)
	}
	_, code := captureStdout(t, func() int {
		return run([]string{"score", "--baseline-rev", "2026-09.1", path})
	})
	if code != 0 {
		t.Fatalf("matching revision pin exit = %d, want 0", code)
	}
}

func TestRunScoreWritesOutFile(t *testing.T) {
	path := scoreTestReport(t)
	outPath := filepath.Join(t.TempDir(), "assessment.json")
	if code := run([]string{"score", "--out", outPath, path}); code != 0 {
		t.Fatalf("score --out exit = %d", code)
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	var header struct {
		ReportKind string `json:"report_kind"`
	}
	if err := json.Unmarshal(data, &header); err != nil {
		t.Fatal(err)
	}
	if header.ReportKind != "assessment" {
		t.Fatalf("out file report_kind = %q", header.ReportKind)
	}
}
