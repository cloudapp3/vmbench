package score

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"
)

// --- fixture helpers -------------------------------------------------------

// runDocument renders a run report. workloadResults maps workload name to the
// raw result object; a nil result records a workload entry without a result.
func runDocument(osName string, iterations int, tools []string, workloadResults map[string]map[string]any) []byte {
	names := sortedKeys(workloadResults)
	rows := make([]map[string]any, 0, len(names))
	for _, name := range names {
		row := map[string]any{"name": name, "category": "test", "description": name}
		if workloadResults[name] != nil {
			row["result"] = workloadResults[name]
		}
		rows = append(rows, row)
	}
	doc := map[string]any{
		"schema_version": 2,
		"version":        "test",
		"system":         map[string]any{"os": map[string]any{"name": osName, "kernel": "6.1.0-test"}},
		"config":         map[string]any{"iterations": iterations, "hardware_tools": tools},
		"results":        map[string]any{"workloads": rows},
	}
	data, err := json.Marshal(doc)
	if err != nil {
		panic(err)
	}
	return data
}

func checkupDocument(reportKind, osName string, hardwareReport []byte, speedSummary map[string]any, pingResults []map[string]any) []byte {
	hardware := map[string]any{"enabled": true, "status": "done"}
	if hardwareReport != nil {
		var embedded map[string]any
		if err := json.Unmarshal(hardwareReport, &embedded); err != nil {
			panic(err)
		}
		hardware["report"] = embedded
	}
	doc := map[string]any{
		"schema_version": 2,
		"report_kind":    reportKind,
		"system":         map[string]any{"os": map[string]any{"name": osName, "kernel": "6.1.0-test"}},
		"config":         map[string]any{"iterations": 3},
		"hardware":       hardware,
		"ping":           map[string]any{"results": pingResults},
	}
	if speedSummary != nil {
		doc["speed"] = map[string]any{"result": map[string]any{"summary": speedSummary}}
	}
	data, err := json.Marshal(doc)
	if err != nil {
		panic(err)
	}
	return data
}

func sortedKeys(values map[string]map[string]any) []string {
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	for i := 1; i < len(names); i++ {
		for j := i; j > 0 && names[j] < names[j-1]; j-- {
			names[j], names[j-1] = names[j-1], names[j]
		}
	}
	return names
}

func throughputRow(unit string, value float64) map[string]any {
	return map[string]any{
		"iterations": 3, "median_ms": 100.0,
		"throughput_per_sec": value, "throughput_unit": unit,
		"samples_ms": []float64{90, 100, 110},
	}
}

func findDimension(t *testing.T, assessment Assessment, id string) DimensionResult {
	t.Helper()
	for _, dimension := range assessment.Dimensions {
		if dimension.ID == id {
			return dimension
		}
	}
	t.Fatalf("dimension %q not found in assessment", id)
	return DimensionResult{}
}

func findMetric(t *testing.T, assessment Assessment, id string) MetricResult {
	t.Helper()
	for _, dimension := range assessment.Dimensions {
		for _, metric := range dimension.Metrics {
			if metric.ID == id {
				return metric
			}
		}
	}
	t.Fatalf("metric %q not found in assessment", id)
	return MetricResult{}
}

func findProfile(t *testing.T, assessment Assessment, id string) ProfileResult {
	t.Helper()
	for _, profile := range assessment.Profiles {
		if profile.ID == id {
			return profile
		}
	}
	t.Fatalf("profile %q not found in assessment", id)
	return ProfileResult{}
}

func mustEvaluate(t *testing.T, data []byte, opts Options) Assessment {
	t.Helper()
	assessment, err := Evaluate(data, opts)
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}
	return assessment
}

// --- normalization ---------------------------------------------------------

func TestNormalizeLogMonotonicAndClamped(t *testing.T) {
	if got := normalizeLog(500, 500, 10000); math.Abs(got-0) > 1e-9 {
		t.Fatalf("normalizeLog at floor = %v, want 0", got)
	}
	if got := normalizeLog(10000, 500, 10000); math.Abs(got-100) > 1e-9 {
		t.Fatalf("normalizeLog at ceiling = %v, want 100", got)
	}
	if got := normalizeLog(100, 500, 10000); got != 0 {
		t.Fatalf("normalizeLog below floor = %v, want 0", got)
	}
	if got := normalizeLog(1e9, 500, 10000); got != 100 {
		t.Fatalf("normalizeLog above ceiling = %v, want 100", got)
	}
	previous := -1.0
	for value := 500.0; value <= 10000; value += 250 {
		got := normalizeLog(value, 500, 10000)
		if got < previous {
			t.Fatalf("normalizeLog not monotonic at %v: %v < %v", value, got, previous)
		}
		previous = got
	}
}

func TestNormalizeBandAnchors(t *testing.T) {
	bands := BandThresholds{Excellent: 1, Good: 3, Fair: 8, Cutoff: 50}
	cases := []struct {
		value float64
		want  float64
	}{
		{0.5, 100},
		{1, 100},
		{3, 70},
		{8, 40},
		{50, 0},
		{51, 0},
		{2, 85},   // midpoint excellent..good
		{5.5, 55}, // midpoint good..fair
	}
	for _, tc := range cases {
		if got := normalizeBand(tc.value, bands); math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("normalizeBand(%v) = %v, want %v", tc.value, got, tc.want)
		}
	}
}

func TestCoefficientOfVariation(t *testing.T) {
	if got := coefficientOfVariation([]float64{100}); got != 0 {
		t.Fatalf("CV of single sample = %v, want 0", got)
	}
	if got := coefficientOfVariation([]float64{100, 100, 100}); got != 0 {
		t.Fatalf("CV of flat samples = %v, want 0", got)
	}
	got := coefficientOfVariation([]float64{90, 110})
	if math.Abs(got-0.1414213562373095) > 1e-9 {
		t.Fatalf("CV of {90,110} = %v, want 0.1414", got)
	}
}

// --- evaluation ------------------------------------------------------------

func fullRunWorkloads() map[string]map[string]any {
	return map[string]map[string]any{
		"CPU Single-Core (sysbench)":        throughputRow("events/sec", 4000),
		"CPU Multi-Core (sysbench)":         throughputRow("events/sec", 40000),
		"OpenSSL AES-256-CBC":               throughputRow("MiB/s", 2000),
		"OpenSSL SHA256":                    throughputRow("MiB/s", 1500),
		"Memory Read Bandwidth (sysbench)":  throughputRow("MiB/s", 12000),
		"Memory Write Bandwidth (sysbench)": throughputRow("MiB/s", 9000),
		"Memory Random Read Latency (sysbench)": {
			"iterations": 3, "median_ms": 5.0,
			"throughput_per_sec": 0, "throughput_unit": "",
			"avg_ns_per_access": 80,
			"samples_ms":        []float64{4.8, 5.0, 5.2},
		},
		"Disk 4K Random Read Q32 (fio)":  throughputRow("IOPS", 80000),
		"Disk 4K Random Write Q32 (fio)": throughputRow("IOPS", 50000),
		"Disk 4K Random Read Q1 (fio)": {
			"iterations": 3, "median_ms": 0.2,
			"throughput_per_sec": 8000, "throughput_unit": "IOPS",
			"latency_p99_ns": 200000,
			"samples_ms":     []float64{0.19, 0.2, 0.21},
		},
		"Disk 4K Random Write Q1 (fio)": {
			"iterations": 3, "median_ms": 0.4,
			"throughput_per_sec": 6000, "throughput_unit": "IOPS",
			"latency_p99_ns": 400000,
			"samples_ms":     []float64{0.39, 0.4, 0.41},
		},
		"Disk 1M Sequential Read Q8 (fio)":  throughputRow("MiB/s", 2500),
		"Disk 1M Sequential Write Q8 (fio)": throughputRow("MiB/s", 1800),
		"Disk Read (dd)":                    throughputRow("MiB/s", 400),
		"Disk Write (dd)":                   throughputRow("MiB/s", 300),
		"Memory Bandwidth (STREAM)":         throughputRow("MB/s", 20000),
		"Memory Bandwidth (mbw)":            throughputRow("MiB/s", 15000),
		"CPU Steal (/proc/stat)":            throughputRow("%", 0.8),
	}
}

var fullRunTools = []string{"sysbench", "openssl", "fio", "dd", "stream", "mbw"}

func TestEvaluateFullRunReportIsCompleteAndDeterministic(t *testing.T) {
	data := runDocument("Ubuntu 24.04 LTS", 3, fullRunTools, fullRunWorkloads())
	assessment := mustEvaluate(t, data, Options{})

	if assessment.Source.Kind != "run" {
		t.Fatalf("source kind = %q, want run", assessment.Source.Kind)
	}
	if assessment.Composite == nil {
		t.Fatalf("composite withheld: warnings %v", assessment.Warnings)
	}
	if assessment.Composite.Status != "complete" {
		t.Fatalf("composite status = %q, want complete", assessment.Composite.Status)
	}
	if assessment.Composite.Index <= 0 || assessment.Composite.Index > 100 {
		t.Fatalf("composite index %v out of range", assessment.Composite.Index)
	}
	for _, dimension := range assessment.Dimensions {
		if dimension.ID == "network" {
			// Network metrics need checkup reports; on runs they are out of scope.
			if dimension.Status != DimensionNotApplicable {
				t.Fatalf("network status = %q, want not_applicable on run reports", dimension.Status)
			}
			continue
		}
		if dimension.Status != DimensionScored {
			t.Fatalf("dimension %s status = %q, want scored", dimension.ID, dimension.Status)
		}
		if len(dimension.Missing) != 0 {
			t.Fatalf("dimension %s missing = %v, want empty", dimension.ID, dimension.Missing)
		}
	}
	if math.Abs(assessment.Coverage.MetricPct-100) > 1e-9 {
		t.Fatalf("coverage = %v, want 100", assessment.Coverage.MetricPct)
	}
	if math.Abs(assessment.Coverage.Confidence-1) > 1e-9 {
		t.Fatalf("confidence = %v, want 1.0 at 3 iterations", assessment.Coverage.Confidence)
	}
	if len(assessment.Warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", assessment.Warnings)
	}
	for _, profile := range assessment.Profiles {
		if profile.Fit == FitUnsuitable {
			t.Fatalf("profile %s unexpectedly vetoed: %+v", profile.ID, profile.Veto)
		}
	}

	// Spot-check one log normalization: 4000 events/s on floor 500, ceiling 10000.
	metric := findMetric(t, assessment, "cpu.sysbench.single")
	want := 100 * math.Log(4000.0/500.0) / math.Log(10000.0/500.0)
	if math.Abs(metric.Normalized-want) > 1e-9 {
		t.Fatalf("cpu.sysbench.single normalized = %v, want %v", metric.Normalized, want)
	}

	// Determinism: identical bytes on repeated evaluation.
	first, err := json.Marshal(assessment)
	if err != nil {
		t.Fatal(err)
	}
	second, err := json.Marshal(mustEvaluate(t, data, Options{}))
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatalf("Evaluate is not deterministic:\n%s\n%s", first, second)
	}
}

func TestEvaluateSparseCPUOnlyWithholdsComposite(t *testing.T) {
	data := runDocument("Debian 12", 3, []string{"sysbench"}, map[string]map[string]any{
		"CPU Single-Core (sysbench)": throughputRow("events/sec", 3000),
		"CPU Multi-Core (sysbench)":  throughputRow("events/sec", 20000),
	})
	assessment := mustEvaluate(t, data, Options{})

	if assessment.Composite != nil {
		t.Fatalf("composite = %+v, want withheld (only one performance dimension)", assessment.Composite)
	}
	found := false
	for _, warning := range assessment.Warnings {
		if strings.Contains(warning, "composite withheld") {
			found = true
		}
	}
	if !found {
		t.Fatalf("warnings %v lack composite-withheld reason", assessment.Warnings)
	}
	cpu := findDimension(t, assessment, "cpu")
	if cpu.Status != DimensionScored {
		t.Fatalf("cpu status = %q, want scored", cpu.Status)
	}
	memory := findDimension(t, assessment, "memory")
	if memory.Status != DimensionNoData {
		t.Fatalf("memory status = %q, want no_data", memory.Status)
	}
	network := findDimension(t, assessment, "network")
	if network.Status != DimensionNotApplicable {
		t.Fatalf("network status = %q, want not_applicable on run reports", network.Status)
	}
	for _, profile := range assessment.Profiles {
		if len(profile.Missing) == 0 {
			t.Fatalf("profile %s missing list empty on sparse report", profile.ID)
		}
	}
}

func TestEvaluateErrorRowsCountAsMissing(t *testing.T) {
	results := map[string]map[string]any{}
	for name, row := range fullRunWorkloads() {
		if strings.Contains(name, "(fio)") {
			results[name] = row
		}
	}
	results["Disk 4K Random Read Q32 (fio)"] = map[string]any{"error": "fio exited 1: permission denied"}
	data := runDocument("Ubuntu 22.04", 3, []string{"fio"}, results)
	assessment := mustEvaluate(t, data, Options{})

	metric := findMetric(t, assessment, "disk.rand4k.read.q32")
	if metric.Status != MetricError {
		t.Fatalf("status = %q, want error", metric.Status)
	}
	if !strings.Contains(metric.Note, "fio exited 1") {
		t.Fatalf("note = %q, want workload error text", metric.Note)
	}
	disk := findDimension(t, assessment, "disk")
	if len(disk.Missing) != 1 || disk.Missing[0] != "disk.rand4k.read.q32" {
		t.Fatalf("disk missing = %v, want [disk.rand4k.read.q32]", disk.Missing)
	}
	if disk.Status != DimensionScored {
		t.Fatalf("disk status = %q, want scored (other fio rows still count)", disk.Status)
	}
	if len(assessment.Coverage.Missing) == 0 {
		t.Fatal("coverage missing list is empty despite failed workload")
	}
}

func TestEvaluateUnitMismatchWarns(t *testing.T) {
	data := runDocument("Ubuntu 22.04", 3, []string{"sysbench"}, map[string]map[string]any{
		"CPU Single-Core (sysbench)": throughputRow("events/s", 4000),
	})
	assessment := mustEvaluate(t, data, Options{})

	metric := findMetric(t, assessment, "cpu.sysbench.single")
	if metric.Status != MetricUnitMismatch {
		t.Fatalf("status = %q, want unit_mismatch", metric.Status)
	}
	found := false
	for _, warning := range assessment.Warnings {
		if strings.Contains(warning, "cpu.sysbench.single") && strings.Contains(warning, "unit mismatch") {
			found = true
		}
	}
	if !found {
		t.Fatalf("warnings %v lack unit mismatch entry", assessment.Warnings)
	}
}

func TestEvaluateLatencyPrefersP99AndDisclosesFallback(t *testing.T) {
	results := map[string]map[string]any{
		"Disk 4K Random Read Q1 (fio)": {
			"iterations": 3, "median_ms": 0.2,
			"throughput_per_sec": 8000, "throughput_unit": "IOPS",
			"latency_p99_ns": 250000, "avg_ns_per_access": 180000,
			"samples_ms": []float64{0.2, 0.2, 0.2},
		},
	}
	assessment := mustEvaluate(t, runDocument("Ubuntu 22.04", 3, []string{"fio"}, results), Options{})
	metric := findMetric(t, assessment, "disk.rand4k.read.q1.lat")
	if metric.Value != 250000 || metric.Note != "" {
		t.Fatalf("p99 preference broken: value %v note %q", metric.Value, metric.Note)
	}

	delete(results["Disk 4K Random Read Q1 (fio)"], "latency_p99_ns")
	assessment = mustEvaluate(t, runDocument("Ubuntu 22.04", 3, []string{"fio"}, results), Options{})
	metric = findMetric(t, assessment, "disk.rand4k.read.q1.lat")
	if metric.Value != 180000 {
		t.Fatalf("fallback value = %v, want 180000", metric.Value)
	}
	if !strings.Contains(metric.Note, "avg_ns_per_access") {
		t.Fatalf("fallback note = %q, want avg disclosure", metric.Note)
	}
}

func TestEvaluateCheckupReportScoresNetworkAndStability(t *testing.T) {
	hardware := runDocument("", 3, []string{"sysbench", "fio"}, map[string]map[string]any{
		"CPU Single-Core (sysbench)":    throughputRow("events/sec", 4200),
		"Disk 4K Random Read Q32 (fio)": throughputRow("IOPS", 90000),
		"Disk 4K Random Read Q1 (fio)": {
			"iterations": 3, "median_ms": 0.2,
			"throughput_per_sec": 8000, "throughput_unit": "IOPS",
			"latency_p99_ns": 220000,
			"samples_ms":     []float64{0.2, 0.2, 0.2},
		},
		"CPU Steal (/proc/stat)": throughputRow("%", 0.4),
	})
	data := checkupDocument("checkup", "Ubuntu 24.04", hardware,
		map[string]any{"download_mbps": 940.0, "upload_mbps": 520.0},
		[]map[string]any{
			{"status": "ok", "avg_latency_ms": 12, "jitter_ms": 1.2, "packet_loss": 0},
			{"status": "ok", "avg_latency_ms": 14, "jitter_ms": 1.8, "packet_loss": 0},
			{"status": "error", "avg_latency_ms": 0, "jitter_ms": 0, "packet_loss": 100},
		})
	assessment := mustEvaluate(t, data, Options{})

	if assessment.Source.Kind != "checkup" {
		t.Fatalf("source kind = %q, want checkup", assessment.Source.Kind)
	}
	network := findDimension(t, assessment, "network")
	if network.Status != DimensionScored {
		t.Fatalf("network status = %q (%v)", network.Status, network.Missing)
	}
	jitter := findMetric(t, assessment, "net.ping.jitter")
	if jitter.Value != 1.5 { // error probes excluded; median of {1.2, 1.8}
		t.Fatalf("jitter median = %v, want 1.5", jitter.Value)
	}
	loss := findMetric(t, assessment, "stability.net.loss")
	if loss.Value != 0 {
		t.Fatalf("loss = %v, want 0 (error probe excluded)", loss.Value)
	}
	if assessment.Composite == nil {
		t.Fatalf("composite withheld: %v", assessment.Warnings)
	}
	if assessment.Composite.Status != "partial" {
		t.Fatalf("composite status = %q, want partial (memory missing)", assessment.Composite.Status)
	}
}

func TestEvaluateLegacySuiteMatchesCheckup(t *testing.T) {
	base := func(kind string) []byte {
		hardware := runDocument("", 3, []string{"fio"}, map[string]map[string]any{
			"Disk 4K Random Read Q32 (fio)": throughputRow("IOPS", 90000),
			"CPU Steal (/proc/stat)":        throughputRow("%", 0.4),
		})
		return checkupDocument(kind, "Ubuntu 24.04", hardware,
			map[string]any{"download_mbps": 800.0, "upload_mbps": 400.0},
			[]map[string]any{{"status": "ok", "avg_latency_ms": 20, "jitter_ms": 2, "packet_loss": 0}})
	}
	checkupAssessment := mustEvaluate(t, base("checkup"), Options{})
	suiteAssessment := mustEvaluate(t, base("suite"), Options{})

	checkupBytes, err := json.Marshal(checkupAssessment)
	if err != nil {
		t.Fatal(err)
	}
	suiteBytes, err := json.Marshal(suiteAssessment)
	if err != nil {
		t.Fatal(err)
	}
	if string(checkupBytes) != string(suiteBytes) {
		t.Fatalf("legacy suite assessment differs from checkup:\n%s\n%s", checkupBytes, suiteBytes)
	}
}

func TestEvaluateFallbackSubstitutionConsumesSource(t *testing.T) {
	results := map[string]map[string]any{
		"Memory Write Bandwidth (sysbench)": throughputRow("MiB/s", 9000),
		"Memory Random Read Latency (sysbench)": {
			"iterations": 3, "median_ms": 5.0,
			"avg_ns_per_access": 85,
			"samples_ms":        []float64{4.9, 5.0, 5.1},
		},
		"Memory Bandwidth (STREAM)": throughputRow("MB/s", 20000),
	}
	// sysbench memory read is missing while STREAM is present.
	tools := []string{"sysbench", "stream"}
	assessment := mustEvaluate(t, runDocument("Ubuntu 24.04", 3, tools, results), Options{})

	read := findMetric(t, assessment, "mem.read.bw")
	if read.Status != MetricOK || read.Value != 20000 || read.Unit != "MB/s" {
		t.Fatalf("mem.read.bw = %+v, want substituted STREAM reading", read)
	}
	if !strings.Contains(read.Note, "via mem.stream") {
		t.Fatalf("substitution note = %q", read.Note)
	}
	stream := findMetric(t, assessment, "mem.stream")
	if !strings.Contains(stream.Note, "consumed by mem.read.bw") {
		t.Fatalf("stream note = %q, want consumed disclosure", stream.Note)
	}
	if stream.Weight != 0 {
		t.Fatalf("consumed stream weight = %v, want 0 (no double counting)", stream.Weight)
	}
	memory := findDimension(t, assessment, "memory")
	if memory.Status != DimensionScored {
		t.Fatalf("memory status = %q, missing %v", memory.Status, memory.Missing)
	}
	if len(memory.Missing) != 0 {
		t.Fatalf("memory missing = %v, want empty (fallback covered)", memory.Missing)
	}
	for _, id := range assessment.Coverage.Present {
		if id == "mem.stream" {
			t.Fatal("consumed fallback listed in coverage present")
		}
	}
}

func TestEvaluateWebVetoOnQ1Latency(t *testing.T) {
	results := fullRunWorkloads()
	results["Disk 4K Random Read Q1 (fio)"]["latency_p99_ns"] = 3000000.0 // > 2ms web veto
	assessment := mustEvaluate(t, runDocument("Ubuntu 24.04", 3, fullRunTools, results), Options{})

	web := findProfile(t, assessment, "web")
	if web.Fit != FitUnsuitable {
		t.Fatalf("web fit = %q, want unsuitable", web.Fit)
	}
	if web.Veto == nil || web.Veto.Metric != "disk.rand4k.read.q1.lat" {
		t.Fatalf("web veto = %+v, want disk.rand4k.read.q1.lat", web.Veto)
	}
	if web.Rating != "C" {
		t.Fatalf("web rating = %q, want capped C", web.Rating)
	}
	storage := findProfile(t, assessment, "storage")
	if storage.Fit == FitUnsuitable {
		t.Fatalf("storage veto unexpectedly triggered: %+v", storage.Veto)
	}
}

func TestEvaluateStorageVetoOnQ32IOPS(t *testing.T) {
	results := fullRunWorkloads()
	results["Disk 4K Random Read Q32 (fio)"] = throughputRow("IOPS", 12000) // < 15000 storage veto
	assessment := mustEvaluate(t, runDocument("Ubuntu 24.04", 3, fullRunTools, results), Options{})

	storage := findProfile(t, assessment, "storage")
	if storage.Fit != FitUnsuitable || storage.Veto == nil || storage.Veto.Metric != "disk.rand4k.read.q32" {
		t.Fatalf("storage = %+v, want q32 iops veto", storage)
	}
	web := findProfile(t, assessment, "web")
	if web.Fit == FitUnsuitable {
		t.Fatalf("web veto unexpectedly triggered: %+v", web.Veto)
	}
}

func TestEvaluatePlatformGating(t *testing.T) {
	results := map[string]map[string]any{
		"CPU Single-Core (sysbench)": throughputRow("events/sec", 3000),
	}
	linux := mustEvaluate(t, runDocument("Ubuntu 24.04", 3, []string{"sysbench"}, results), Options{})
	steal := findMetric(t, linux, "stability.cpu.steal")
	if steal.Status != MetricAbsent {
		t.Fatalf("steal status on linux = %q, want absent", steal.Status)
	}
	for _, dimension := range linux.Dimensions {
		for _, metric := range dimension.Metrics {
			if metric.ID == "cpu.winsat" {
				t.Fatalf("winsat metric applicable on linux report: %+v", metric)
			}
		}
	}

	windows := mustEvaluate(t, runDocument("Microsoft Windows 11 Pro", 3, []string{"sysbench"}, results), Options{})
	for _, dimension := range windows.Dimensions {
		for _, metric := range dimension.Metrics {
			if metric.ID == "stability.cpu.steal" {
				t.Fatalf("steal metric present on windows report: %+v", metric)
			}
		}
	}
}

func TestEvaluateRevisionPinMismatchFails(t *testing.T) {
	data := runDocument("Ubuntu 24.04", 3, []string{"sysbench"}, map[string]map[string]any{
		"CPU Single-Core (sysbench)": throughputRow("events/sec", 3000),
	})
	if _, err := Evaluate(data, Options{RequireRevision: "1999.1"}); err == nil {
		t.Fatal("Evaluate accepted a mismatched required revision")
	}
	assessment := mustEvaluate(t, data, Options{RequireRevision: "2026-09.1"})
	if assessment.Baseline.Revision != "2026-09.1" {
		t.Fatalf("revision = %q", assessment.Baseline.Revision)
	}
}

func TestEvaluateIterationConfidenceDiscount(t *testing.T) {
	workloads := map[string]map[string]any{
		"CPU Single-Core (sysbench)": throughputRow("events/sec", 3000),
		"CPU Multi-Core (sysbench)":  throughputRow("events/sec", 20000),
	}
	assessment := mustEvaluate(t, runDocument("Ubuntu 24.04", 1, []string{"sysbench"}, workloads), Options{})
	want := percentToFraction(assessment.Coverage.MetricPct) * 0.7
	if math.Abs(assessment.Coverage.Confidence-want) > 1e-9 {
		t.Fatalf("confidence at 1 iteration = %v, want %v (coverage × 0.7)", assessment.Coverage.Confidence, want)
	}
}

// --- input extraction ------------------------------------------------------

func TestInferPlatform(t *testing.T) {
	cases := []struct {
		name, kernel, want string
	}{
		{"Ubuntu 24.04 LTS", "6.8.0-40-generic", "linux"},
		{"Microsoft Windows 11 Pro", "10.0.22631", "windows"},
		{"macOS 14.6", "Darwin 23.6.0", "darwin"},
		{"", "", ""},
	}
	for _, tc := range cases {
		if got := inferPlatform(tc.name, tc.kernel); got != tc.want {
			t.Errorf("inferPlatform(%q, %q) = %q, want %q", tc.name, tc.kernel, got, tc.want)
		}
	}
}

func TestEvaluateRejectsNonReportJSON(t *testing.T) {
	if _, err := Evaluate([]byte(`{"hello": "world"}`), Options{}); err == nil {
		t.Fatal("Evaluate accepted non-report JSON")
	}
}

// --- baseline loading ------------------------------------------------------

func TestEmbeddedBaselineDecodes(t *testing.T) {
	baseline, err := EmbeddedBaseline()
	if err != nil {
		t.Fatalf("embedded baseline invalid: %v", err)
	}
	if baseline.Revision == "" || baseline.SchemaVersion != 1 {
		t.Fatalf("unexpected baseline header: %+v", baseline)
	}
	if _, ok := baseline.Metrics["cpu.sysbench.single"]; !ok {
		t.Fatal("embedded baseline lacks cpu.sysbench.single")
	}
}

func TestDecodeBaselineStrictness(t *testing.T) {
	valid := []byte(`{
		"schema_version": 1,
		"revision": "test",
		"grades": {"s": 90, "a": 80, "b": 65, "c": 50},
		"metrics": {
			"m1": {
				"dimension": "cpu", "weight": 1, "source": {"workload": "W", "field": "throughput_per_sec"},
				"class": "log", "floor": 1, "ceiling": 10
			}
		},
		"dimensions": {"cpu": {"weight": 1}},
		"profiles": {}
	}`)
	if _, err := DecodeBaseline(valid); err != nil {
		t.Fatalf("valid baseline rejected: %v", err)
	}

	unknown := strings.Replace(string(valid), `"revision": "test",`, `"revision": "test", "extra": true,`, 1)
	if _, err := DecodeBaseline([]byte(unknown)); err == nil {
		t.Fatal("unknown baseline field accepted")
	}

	if _, err := DecodeBaseline(append(append([]byte{}, valid...), []byte(` {"schema_version":1}`)...)); err == nil {
		t.Fatal("trailing JSON accepted")
	}

	if _, err := DecodeBaseline(make([]byte, maxBaselineBytes+1)); err == nil {
		t.Fatal("oversized baseline accepted")
	}
}

func TestBaselineValidateRejections(t *testing.T) {
	validMetric := map[string]any{
		"dimension": "cpu", "weight": 1,
		"source": map[string]any{"workload": "W", "field": "throughput_per_sec"},
		"class":  "log", "floor": 1, "ceiling": 10,
	}
	build := func(metrics map[string]any, dimensions map[string]any, profiles map[string]any) []byte {
		doc := map[string]any{
			"schema_version": 1, "revision": "t",
			"grades":     map[string]any{"s": 90, "a": 80, "b": 65, "c": 50},
			"metrics":    metrics,
			"dimensions": dimensions,
			"profiles":   profiles,
		}
		data, _ := json.Marshal(doc)
		return data
	}
	cpu := func() map[string]any { return map[string]any{"cpu": map[string]any{"weight": 1}} }

	cases := map[string][]byte{
		"unknown dimension reference": build(map[string]any{
			"m1": map[string]any{"dimension": "gpu", "weight": 1, "source": validMetric["source"], "class": "log", "floor": 1, "ceiling": 10},
		}, cpu(), map[string]any{}),
		"dimension weight sum": build(
			map[string]any{"m1": validMetric},
			map[string]any{"cpu": map[string]any{"weight": 0.5}, "memory": map[string]any{"weight": 0.4}},
			map[string]any{},
		),
		"band ordering": build(map[string]any{
			"m1": map[string]any{
				"dimension": "cpu", "weight": 1,
				"source": map[string]any{"workload": "W", "field": "throughput_per_sec"},
				"class":  "band", "bands": map[string]any{"excellent": 10, "good": 5, "fair": 3, "cutoff": 1},
			},
		}, cpu(), map[string]any{}),
		"unknown veto metric": build(
			map[string]any{"m1": validMetric},
			cpu(),
			map[string]any{
				"web": map[string]any{
					"weights": map[string]any{"cpu": 1},
					"vetoes":  []map[string]any{{"metric": "nope", "op": "gt", "value": 1}},
				},
			},
		),
		"unknown requires_kind": build(map[string]any{
			"m1": map[string]any{"dimension": "cpu", "weight": 1, "source": validMetric["source"], "class": "log", "floor": 1, "ceiling": 10, "requires_kind": "banana"},
		}, cpu(), map[string]any{}),
	}
	for name, data := range cases {
		if _, err := DecodeBaseline(data); err == nil {
			t.Errorf("%s: invalid baseline accepted", name)
		}
	}
}

// profile weight sum rejection needs a profile whose weights do not sum to 1.
func TestBaselineValidateProfileWeightSum(t *testing.T) {
	doc := map[string]any{
		"schema_version": 1, "revision": "t",
		"grades": map[string]any{"s": 90, "a": 80, "b": 65, "c": 50},
		"metrics": map[string]any{
			"m1": map[string]any{
				"dimension": "cpu", "weight": 1,
				"source": map[string]any{"workload": "W", "field": "throughput_per_sec"},
				"class":  "log", "floor": 1, "ceiling": 10,
			},
		},
		"dimensions": map[string]any{"cpu": map[string]any{"weight": 1}},
		"profiles": map[string]any{
			"web": map[string]any{"weights": map[string]any{"cpu": 0.5, "memory": 0.2}},
		},
	}
	data, _ := json.Marshal(doc)
	if _, err := DecodeBaseline(data); err == nil {
		t.Fatal("profile weights summing to 0.7 accepted")
	}
}

func TestIterationFactor(t *testing.T) {
	cases := map[int]float64{1: 0.7, 2: 0.85, 3: 1.0, 5: 1.0}
	for iterations, want := range cases {
		if got := iterationFactor(iterations); got != want {
			t.Errorf("iterationFactor(%d) = %v, want %v", iterations, got, want)
		}
	}
}

func TestCapRating(t *testing.T) {
	cases := []struct {
		rating, cap, want string
	}{
		{"S", "C", "C"},
		{"A", "C", "C"},
		{"B", "C", "C"},
		{"C", "C", "C"},
		{"D", "C", "D"},
		{"B", "", "B"},
	}
	for _, tc := range cases {
		if got := capRating(tc.rating, tc.cap); got != tc.want {
			t.Errorf("capRating(%q, %q) = %q, want %q", tc.rating, tc.cap, got, tc.want)
		}
	}
}

func TestSortedByCanonicalOrder(t *testing.T) {
	values := map[string]int{"stability": 1, "cpu": 2, "zeta": 3, "network": 4, "alpha": 5}
	got := sortedByCanonicalOrder(values, dimensionOrder)
	want := []string{"cpu", "network", "stability", "alpha", "zeta"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("sortedByCanonicalOrder = %v, want %v", got, want)
	}
}
