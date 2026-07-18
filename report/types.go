package report

import (
	"time"

	"github.com/cloudapp3/vmbench/bench"
	"github.com/cloudapp3/vmbench/sysinfo"
)

// ResultEntry stores one workload result inside a report document.
type ResultEntry struct {
	Iterations       int       `json:"iterations,omitempty"`
	MedianMS         float64   `json:"median_ms"`
	SamplesMS        []float64 `json:"samples_ms,omitempty"`
	ThroughputPerSec float64   `json:"throughput_per_sec"`
	ThroughputUnit   string    `json:"throughput_unit"`
	AvgNSPerAccess   float64   `json:"avg_ns_per_access,omitempty"`
	BytesProcessed   int64     `json:"bytes_processed,omitempty"`
	OpsProcessed     float64   `json:"ops_processed,omitempty"`
	Detail           string    `json:"detail,omitempty"`
	Error            string    `json:"error,omitempty"`
}

// WorkloadEntry stores one workload report row.
type WorkloadEntry struct {
	Name        string       `json:"name"`
	Category    string       `json:"category"`
	Description string       `json:"description"`
	Result      *ResultEntry `json:"result,omitempty"`
}

// ResultsSection stores benchmark workload results without synthetic scoring.
type ResultsSection struct {
	Workloads []WorkloadEntry `json:"workloads"`
}

// ExtensionsSection stores optional extension workloads.
type ExtensionsSection struct {
	Workloads []WorkloadEntry `json:"workloads,omitempty"`
}

// RunConfig describes the CLI configuration used to produce a report.
type RunConfig struct {
	Iterations      int      `json:"iterations"`
	Filter          string   `json:"filter,omitempty"`
	DiskPath        string   `json:"disk_path,omitempty"`
	Extensions      bool     `json:"extensions"`
	Mode            string   `json:"mode,omitempty"`
	Scope           string   `json:"scope,omitempty"`
	HardwareTools   []string `json:"hardware_tools,omitempty"`
	IperfHosts      []string `json:"iperf_hosts,omitempty"`
	CatalogSource   string   `json:"catalog_source,omitempty"`
	CatalogRevision string   `json:"catalog_revision,omitempty"`
	NodeIDs         []string `json:"node_ids,omitempty"`
}

// Document is the full report representation shared by JSON and HTML outputs.
type Document struct {
	SchemaVersion int                `json:"schema_version"`
	Version       string             `json:"version"`
	Timestamp     time.Time          `json:"timestamp"`
	System        sysinfo.SystemInfo `json:"system"`
	Config        RunConfig          `json:"config"`
	Results       ResultsSection     `json:"results"`
	Extensions    ExtensionsSection  `json:"extensions,omitempty"`
	Warnings      []string           `json:"warnings,omitempty"`
}

const currentSchemaVersion = 2

// BuildDocument converts benchmark results into a report document.
func BuildDocument(version string, system sysinfo.SystemInfo, cfg RunConfig, results []bench.BenchResult, warnings []string) Document {
	core := make([]WorkloadEntry, 0, len(results))
	ext := make([]WorkloadEntry, 0, 4)
	for _, result := range results {
		entry := WorkloadEntry{
			Name:        result.Workload,
			Category:    result.Category,
			Description: result.Description,
			Result:      convertDetail(result.Result),
		}
		if isExtensionCategory(result.Category) {
			ext = append(ext, entry)
			continue
		}
		core = append(core, entry)
	}
	return Document{
		SchemaVersion: currentSchemaVersion,
		Version:       version,
		Timestamp:     time.Now().UTC(),
		System:        system,
		Config:        cfg,
		Results:       ResultsSection{Workloads: core},
		Extensions:    ExtensionsSection{Workloads: ext},
		Warnings:      append([]string(nil), warnings...),
	}
}

func convertDetail(detail *bench.RunDetail) *ResultEntry {
	if detail == nil {
		return nil
	}
	samplesMS := make([]float64, 0, len(detail.Samples))
	for _, sample := range detail.Samples {
		samplesMS = append(samplesMS, float64(sample)/float64(time.Millisecond))
	}
	return &ResultEntry{
		Iterations:       detail.Iterations,
		MedianMS:         float64(detail.MedianTime) / float64(time.Millisecond),
		SamplesMS:        samplesMS,
		ThroughputPerSec: detail.Throughput,
		ThroughputUnit:   detail.ThroughputUnit,
		AvgNSPerAccess:   detail.AverageLatencyNS,
		BytesProcessed:   detail.BytesProcessed,
		OpsProcessed:     detail.OpsProcessed,
		Detail:           detail.Detail,
		Error:            detail.Error,
	}
}

// HasFailures reports whether a document has no workloads or any workload failed.
func HasFailures(doc Document) bool {
	workloads := make([]WorkloadEntry, 0, len(doc.Results.Workloads)+len(doc.Extensions.Workloads))
	workloads = append(workloads, doc.Results.Workloads...)
	workloads = append(workloads, doc.Extensions.Workloads...)
	if len(workloads) == 0 {
		return true
	}
	for _, workload := range workloads {
		if workload.Result == nil || workload.Result.Error != "" {
			return true
		}
	}
	return false
}

func isExtensionCategory(category string) bool {
	return category == bench.CategoryExtensionDisk ||
		category == bench.CategoryNetwork
}
