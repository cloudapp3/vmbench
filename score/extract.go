package score

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/cloudapp3/vmbench/history"
)

// Report kinds accepted by Evaluate. Legacy suite reports are handled as
// checkup reports; they share the checkup wire format.
const (
	kindRun     = "run"
	kindCheckup = "checkup"
)

// input is the normalized view Evaluate extracts from one report document.
type input struct {
	Kind       string
	Platform   string // "linux", "darwin", "windows", inferred from system.os
	Iterations int
	Tools      []string
	Workloads  map[string]workloadRow
	Checkup    checkupEvidence
}

// workloadRow is one report workload entry reduced to score-relevant fields.
type workloadRow struct {
	Error      string
	Unit       string
	Throughput float64
	AvgNS      float64
	P99NS      float64
	SamplesMS  []float64
}

// checkupEvidence aggregates the checkup-only section data that is not tied to
// a single workload row.
type checkupEvidence struct {
	downloadMbps float64
	uploadMbps   float64
	jitterMS     []float64
	latencyMS    []float64
	packetLoss   []float64
	hasSpeedDown bool
	hasSpeedUp   bool
}

// rawRunShape mirrors the report.Document JSON that score needs.
type rawRunShape struct {
	System struct {
		OS struct {
			Name   string `json:"name"`
			Kernel string `json:"kernel"`
		} `json:"os"`
	} `json:"system"`
	Config struct {
		Iterations    int      `json:"iterations"`
		HardwareTools []string `json:"hardware_tools"`
	} `json:"config"`
	Results struct {
		Workloads []rawWorkloadShape `json:"workloads"`
	} `json:"results"`
	Extensions struct {
		Workloads []rawWorkloadShape `json:"workloads"`
	} `json:"extensions"`
}

type rawWorkloadShape struct {
	Name   string       `json:"name"`
	Result *rawRowShape `json:"result"`
}

type rawRowShape struct {
	Iterations       int       `json:"iterations"`
	ThroughputPerSec float64   `json:"throughput_per_sec"`
	ThroughputUnit   string    `json:"throughput_unit"`
	AvgNSPerAccess   float64   `json:"avg_ns_per_access"`
	LatencyP99NS     float64   `json:"latency_p99_ns"`
	SamplesMS        []float64 `json:"samples_ms"`
	Error            string    `json:"error"`
}

// rawCheckupShape mirrors the CheckupReport JSON that score needs.
type rawCheckupShape struct {
	System struct {
		OS struct {
			Name   string `json:"name"`
			Kernel string `json:"kernel"`
		} `json:"os"`
	} `json:"system"`
	Config struct {
		Iterations    int      `json:"iterations"`
		HardwareTools []string `json:"hardware_tools"`
	} `json:"config"`
	Hardware struct {
		Status string       `json:"status"`
		Report *rawRunShape `json:"report"`
	} `json:"hardware"`
	Ping struct {
		Results []struct {
			Status       string  `json:"status"`
			AvgLatencyMs float64 `json:"avg_latency_ms"`
			JitterMs     float64 `json:"jitter_ms"`
			PacketLoss   float64 `json:"packet_loss"`
		} `json:"results"`
	} `json:"ping"`
	Speed struct {
		Result *struct {
			Summary *struct {
				DownloadMbps float64 `json:"download_mbps"`
				UploadMbps   float64 `json:"upload_mbps"`
			} `json:"summary"`
		} `json:"result"`
	} `json:"speed"`
}

// extractInput identifies the report kind and reduces the document to the
// fields Evaluate consumes.
func extractInput(data []byte) (input, error) {
	meta, err := history.Inspect(data)
	if err != nil {
		return input{}, fmt.Errorf("score: %w", err)
	}
	kind := string(meta.Kind)
	if kind == "suite" { // legacy suite reports use the checkup shape
		kind = kindCheckup
	}
	switch kind {
	case kindRun:
		return extractRun(data)
	case kindCheckup:
		return extractCheckup(data)
	default:
		return input{}, fmt.Errorf("score: unsupported report kind %q", kind)
	}
}

func extractRun(data []byte) (input, error) {
	var doc rawRunShape
	if err := json.Unmarshal(data, &doc); err != nil {
		return input{}, fmt.Errorf("score: decoding run report: %w", err)
	}
	in := input{
		Kind:       kindRun,
		Platform:   inferPlatform(doc.System.OS.Name, doc.System.OS.Kernel),
		Iterations: doc.Config.Iterations,
		Tools:      doc.Config.HardwareTools,
		Workloads:  map[string]workloadRow{},
	}
	in.addWorkloads(doc.Results.Workloads)
	in.addWorkloads(doc.Extensions.Workloads)
	return in, nil
}

func extractCheckup(data []byte) (input, error) {
	var doc rawCheckupShape
	if err := json.Unmarshal(data, &doc); err != nil {
		return input{}, fmt.Errorf("score: decoding checkup report: %w", err)
	}
	in := input{
		Kind:       kindCheckup,
		Platform:   inferPlatform(doc.System.OS.Name, doc.System.OS.Kernel),
		Iterations: doc.Config.Iterations,
		Tools:      doc.Config.HardwareTools,
		Workloads:  map[string]workloadRow{},
	}
	if doc.Hardware.Report != nil {
		if in.Iterations == 0 {
			in.Iterations = doc.Hardware.Report.Config.Iterations
		}
		if len(in.Tools) == 0 {
			in.Tools = doc.Hardware.Report.Config.HardwareTools
		}
		in.addWorkloads(doc.Hardware.Report.Results.Workloads)
		in.addWorkloads(doc.Hardware.Report.Extensions.Workloads)
	}
	if doc.Speed.Result != nil && doc.Speed.Result.Summary != nil {
		in.Checkup.downloadMbps = doc.Speed.Result.Summary.DownloadMbps
		in.Checkup.uploadMbps = doc.Speed.Result.Summary.UploadMbps
		in.Checkup.hasSpeedDown = doc.Speed.Result.Summary.DownloadMbps > 0
		in.Checkup.hasSpeedUp = doc.Speed.Result.Summary.UploadMbps > 0
	}
	for _, probe := range doc.Ping.Results {
		if probe.Status != "ok" {
			continue
		}
		if probe.JitterMs > 0 {
			in.Checkup.jitterMS = append(in.Checkup.jitterMS, probe.JitterMs)
		}
		if probe.AvgLatencyMs > 0 {
			in.Checkup.latencyMS = append(in.Checkup.latencyMS, probe.AvgLatencyMs)
		}
		in.Checkup.packetLoss = append(in.Checkup.packetLoss, probe.PacketLoss)
	}
	return in, nil
}

func (in *input) addWorkloads(rows []rawWorkloadShape) {
	for _, row := range rows {
		if row.Name == "" {
			continue
		}
		entry := workloadRow{}
		if row.Result != nil {
			entry.Error = row.Result.Error
			entry.Unit = row.Result.ThroughputUnit
			entry.Throughput = row.Result.ThroughputPerSec
			entry.AvgNS = row.Result.AvgNSPerAccess
			entry.P99NS = row.Result.LatencyP99NS
			entry.SamplesMS = row.Result.SamplesMS
		} else {
			entry.Error = "no result recorded"
		}
		in.Workloads[row.Name] = entry
	}
}

// inferPlatform maps a report's OS identification onto linux/darwin/windows.
// Reports produced by vmbench always carry an OS product name; anything that
// is recognizably not Windows or macOS is treated as Linux, and an empty
// identification stays unknown.
func inferPlatform(name, kernel string) string {
	haystack := strings.ToLower(name + " " + kernel)
	switch {
	case strings.Contains(haystack, "windows"):
		return "windows"
	case strings.Contains(haystack, "darwin"), strings.Contains(haystack, "mac os"), strings.Contains(haystack, "macos"):
		return "darwin"
	case strings.TrimSpace(name) != "":
		return "linux"
	default:
		return ""
	}
}

// metricValue is one metric's extracted raw reading.
type metricValue struct {
	Value float64
	Unit  string
	Note  string
	Found bool
}

// resolveField reads the baseline-declared field for one metric from the
// extracted input. Checkup-sourced fields (empty workload) aggregate the
// matching section evidence; workload rows resolve declared fields with an
// optional fallback and enforce the declared unit list.
func (in input) resolveField(metric MetricBaseline) metricValue {
	if metric.Source.Workload == "" {
		return in.resolveCheckupField(metric)
	}
	row, ok := in.Workloads[metric.Source.Workload]
	if !ok {
		return metricValue{}
	}
	if row.Error != "" {
		return metricValue{Note: "workload error: " + row.Error}
	}
	value := in.rowField(row, metric.Source.Field)
	if value == nil && metric.Source.FallbackField != "" {
		fallback := in.rowField(row, metric.Source.FallbackField)
		if fallback != nil && fallback.Value > 0 {
			fallback.Note = "using " + metric.Source.FallbackField + " (" + metric.Source.Field + " unavailable)"
			return *fallback
		}
	}
	if value == nil {
		return metricValue{}
	}
	if !unitsAccept(metric.Units, value.Unit) {
		return metricValue{Note: "unit mismatch: got " + unitLabel(value.Unit)}
	}
	return *value
}

// rowField reads one declared field from a workload row. The returned unit is
// empty for fields with an intrinsic unit declared by the baseline.
func (in input) rowField(row workloadRow, field string) *metricValue {
	switch field {
	case "throughput_per_sec":
		if row.Throughput <= 0 {
			return nil
		}
		return &metricValue{Value: row.Throughput, Unit: row.Unit, Found: true}
	case "avg_ns_per_access":
		if row.AvgNS <= 0 {
			return nil
		}
		return &metricValue{Value: row.AvgNS, Unit: "ns", Found: true}
	case "latency_p99_ns":
		if row.P99NS <= 0 {
			return nil
		}
		return &metricValue{Value: row.P99NS, Unit: "ns", Found: true}
	default:
		return nil
	}
}

func (in input) resolveCheckupField(metric MetricBaseline) metricValue {
	switch metric.Source.Field {
	case "download_mbps":
		if !in.Checkup.hasSpeedDown {
			return metricValue{}
		}
		return metricValue{Value: in.Checkup.downloadMbps, Unit: "Mbps", Found: true}
	case "upload_mbps":
		if !in.Checkup.hasSpeedUp {
			return metricValue{}
		}
		return metricValue{Value: in.Checkup.uploadMbps, Unit: "Mbps", Found: true}
	case "jitter_ms":
		if len(in.Checkup.jitterMS) == 0 {
			return metricValue{}
		}
		return metricValue{Value: medianFloat(in.Checkup.jitterMS), Unit: "ms", Found: true}
	case "avg_latency_ms":
		if len(in.Checkup.latencyMS) == 0 {
			return metricValue{}
		}
		return metricValue{Value: medianFloat(in.Checkup.latencyMS), Unit: "ms", Found: true}
	case "packet_loss":
		if len(in.Checkup.packetLoss) == 0 {
			return metricValue{}
		}
		return metricValue{Value: medianFloat(in.Checkup.packetLoss), Unit: "fraction", Found: true}
	case "samples_cv":
		cvs := in.sampleCVs()
		if len(cvs) == 0 {
			return metricValue{}
		}
		return metricValue{Value: medianFloat(cvs), Unit: "ratio", Found: true}
	default:
		return metricValue{}
	}
}

// sampleCVs returns the coefficient of variation of every workload row that
// recorded at least two samples, in sorted order for determinism.
func (in input) sampleCVs() []float64 {
	names := make([]string, 0, len(in.Workloads))
	for name := range in.Workloads {
		names = append(names, name)
	}
	sort.Strings(names)
	cvs := make([]float64, 0, len(names))
	for _, name := range names {
		row := in.Workloads[name]
		if row.Error != "" || len(row.SamplesMS) < 2 {
			continue
		}
		if cv := coefficientOfVariation(row.SamplesMS); cv > 0 {
			cvs = append(cvs, cv)
		}
	}
	return cvs
}

// unitsAccept reports whether a row's unit is one the baseline declared. An
// empty declared list accepts any unit (band metrics with intrinsic units).
func unitsAccept(declared []string, unit string) bool {
	if len(declared) == 0 {
		return true
	}
	for _, allowed := range declared {
		if allowed == unit {
			return true
		}
	}
	return false
}

func unitLabel(unit string) string {
	if unit == "" {
		return "(none)"
	}
	return unit
}
