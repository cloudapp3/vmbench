package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudapp3/vmbench/history"
)

func TestParseHistoryAddArgsSupportsTrailingTag(t *testing.T) {
	path, tag, help, err := parseHistoryAddArgs([]string{"report.json", "--tag", "baseline"})
	if err != nil || help || path != "report.json" || tag != "baseline" {
		t.Fatalf("parseHistoryAddArgs = %q %q %t %v", path, tag, help, err)
	}
	path, tag, help, err = parseHistoryAddArgs([]string{"--tag=candidate", "report.json"})
	if err != nil || help || path != "report.json" || tag != "candidate" {
		t.Fatalf("parseHistoryAddArgs equals form = %q %q %t %v", path, tag, help, err)
	}
	if _, _, _, err := parseHistoryAddArgs([]string{"a.json", "b.json"}); err == nil {
		t.Fatal("parseHistoryAddArgs accepted multiple paths")
	}
}

func TestWriteReportComparisonDetectsKindsAndRejectsMixed(t *testing.T) {
	runA := []byte(`{"timestamp":"2026-07-12T01:00:00Z","system":{},"config":{},"results":{"workloads":[]}}`)
	runB := []byte(`{"timestamp":"2026-07-13T01:00:00Z","system":{},"config":{},"results":{"workloads":[]}}`)
	suiteA := []byte(`{"version":1,"config":{},"hardware":{"enabled":true,"status":"ok"}}`)
	suiteB := []byte(`{"report_kind":"suite","config":{},"hardware":{"enabled":true,"status":"ok"}}`)

	var output bytes.Buffer
	if err := writeReportComparison(&output, [][]byte{runA, runB}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "VMBench Compare") {
		t.Fatalf("run comparison output = %s", output.String())
	}
	output.Reset()
	if err := writeReportComparison(&output, [][]byte{suiteA, suiteB}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "VMBench Suite Compare") {
		t.Fatalf("suite comparison output = %s", output.String())
	}
	if err := writeReportComparison(&output, [][]byte{runA, suiteA}); err == nil || !strings.Contains(err.Error(), "mixed report kinds") {
		t.Fatalf("mixed comparison error = %v", err)
	}
}

func TestHistoryCompareRejectsMixedLatestReports(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("VMBENCH_HISTORY_DIR", dir)
	store, err := history.Open("")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Add([]byte(`{"timestamp":"2026-07-12T01:00:00Z","results":{"workloads":[]}}`), "run"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Add([]byte(`{"report_kind":"suite","started_at":"2026-07-13T01:00:00Z","config":{},"hardware":{"enabled":true}}`), "suite"); err != nil {
		t.Fatal(err)
	}
	if code := run([]string{"history", "compare", "--last", "2"}); code != 2 {
		t.Fatalf("history compare mixed kinds exit = %d, want 2", code)
	}
}

func TestHistoryCLILifecycleAndSuiteCompare(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("VMBENCH_HISTORY_DIR", filepath.Join(dir, "history"))
	basePath := filepath.Join(dir, "base.json")
	candidatePath := filepath.Join(dir, "candidate.json")
	writeSuiteHistoryFixture(t, basePath, "suite-base", "2026-07-15T01:00:00Z", 20)
	writeSuiteHistoryFixture(t, candidatePath, "suite-candidate", "2026-07-16T01:00:00Z", 10)

	output, code := captureStdout(t, func() int {
		return run([]string{"history", "add", basePath, "--tag", "baseline"})
	})
	if code != 0 {
		t.Fatalf("history add baseline exit = %d", code)
	}
	baseID := strings.TrimSpace(output)
	if baseID == "" {
		t.Fatal("history add baseline returned an empty ID")
	}

	output, code = captureStdout(t, func() int {
		return run([]string{"history", "add", "--tag=candidate", candidatePath})
	})
	if code != 0 {
		t.Fatalf("history add candidate exit = %d", code)
	}
	candidateID := strings.TrimSpace(output)
	if candidateID == "" || candidateID == baseID {
		t.Fatalf("candidate history ID = %q, baseline = %q", candidateID, baseID)
	}

	output, code = captureStdout(t, func() int { return run([]string{"history", "list"}) })
	if code != 0 || !strings.Contains(output, baseID) || !strings.Contains(output, candidateID) ||
		!strings.Contains(output, "baseline") || !strings.Contains(output, "candidate") {
		t.Fatalf("history list exit = %d, output = %s", code, output)
	}

	output, code = captureStdout(t, func() int {
		return run([]string{"history", "show", candidateID})
	})
	if code != 0 || !strings.Contains(output, `"report_id": "suite-candidate"`) {
		t.Fatalf("history show exit = %d, output = %s", code, output)
	}

	output, code = captureStdout(t, func() int {
		return run([]string{"history", "compare", "--last", "2"})
	})
	for _, want := range []string{"VMBench Suite Compare", "node-a/latency", "▲+50.0%"} {
		if code != 0 || !strings.Contains(output, want) {
			t.Fatalf("history compare exit = %d, output missing %q:\n%s", code, want, output)
		}
	}

	output, code = captureStdout(t, func() int {
		return run([]string{"history", "delete", baseID})
	})
	if code != 0 || !strings.Contains(output, "deleted "+baseID) {
		t.Fatalf("history delete exit = %d, output = %s", code, output)
	}
	store, err := history.Open("")
	if err != nil {
		t.Fatal(err)
	}
	records, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].ID != candidateID {
		t.Fatalf("history records after delete = %+v", records)
	}
}

func TestRunHardwareOnlyOmitsCatalogAndNetworkProvenance(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hardware.json")
	_, code := captureStdout(t, func() int {
		return run([]string{
			"run",
			"--scope", "hardware",
			"--filter", "^definitely-no-workload$",
			"--iterations", "1",
			"--quiet",
			"--node-catalog", "auto",
			"--node-revision", "unused-revision",
			"--node-cache", filepath.Join(t.TempDir(), "unused-cache.json"),
			"--json", path,
		})
	})
	if code != 1 {
		t.Fatalf("hardware-only empty run exit = %d, want 1 for no workloads", code)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var report struct {
		Config map[string]json.RawMessage `json:"config"`
	}
	if err := json.Unmarshal(raw, &report); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"catalog_source", "catalog_revision", "node_ids", "iperf_hosts"} {
		if _, exists := report.Config[key]; exists {
			t.Fatalf("hardware-only config unexpectedly contains %q: %s", key, raw)
		}
	}
	if string(report.Config["scope"]) != `"hardware"` || string(report.Config["extensions"]) != "false" {
		t.Fatalf("hardware-only config = %s", raw)
	}
}

func writeSuiteHistoryFixture(t *testing.T, path, reportID, startedAt string, latency int) {
	t.Helper()
	report := fmt.Sprintf(`{
  "schema_version": 2,
  "report_kind": "suite",
  "report_id": %q,
  "started_at": %q,
  "catalog_revision": "catalog-1",
  "config": {},
  "ping": {"enabled": true, "status": "ok", "results": [
    {"id": "node-a", "target": "ping.example", "ip_family": "v4", "protocol": "tcp", "source": "node-catalog", "status": "ok", "avg_latency_ms": %d}
  ]}
}`, reportID, startedAt, latency)
	if err := os.WriteFile(path, []byte(report), 0o600); err != nil {
		t.Fatal(err)
	}
}

func captureStdout(t *testing.T, fn func() int) (string, int) {
	t.Helper()
	original := os.Stdout
	file, err := os.CreateTemp(t.TempDir(), "stdout-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		os.Stdout = original
		_ = file.Close()
	}()
	os.Stdout = file
	code := fn()
	os.Stdout = original
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := os.ReadFile(file.Name())
	if err != nil {
		t.Fatal(err)
	}
	return string(output), code
}
