package bench

import (
	"context"
	"time"
)

const (
	// CategoryInteger groups integer-heavy workloads.
	CategoryInteger = "Integer"
	// CategoryFloat groups floating-point workloads.
	CategoryFloat = "Float"
	// CategoryMemory groups memory workloads.
	CategoryMemory = "Memory"
	// CategoryExtensionDisk groups disk extension workloads.
	CategoryExtensionDisk = "Extension/Disk"
	// CategoryNetwork groups network speed-test workloads.
	CategoryNetwork = "Network"
)

// Workload describes a deterministic benchmark task.
type Workload interface {
	Name() string
	Category() string
	Description() string
	Run(ctx context.Context) (elapsed time.Duration, processed int64, err error)
	Validate() error
}

// ProcessedKind describes the cumulative quantity returned by Workload.Run.
// Rate, score, latency, and diagnostic values must remain ProcessedUnknown.
type ProcessedKind uint8

const (
	ProcessedUnknown ProcessedKind = iota
	ProcessedBytes
	ProcessedOperations
)

// ProcessedMetricReporter explicitly identifies the semantic meaning of the
// processed value returned by the most recent successful Workload.Run call.
// Workloads that return a rate or another non-cumulative value should either
// omit this interface or return ProcessedUnknown.
type ProcessedMetricReporter interface {
	ProcessedKind() ProcessedKind
}

// CloneableWorkload can create an isolated copy for repeated execution.
type CloneableWorkload interface {
	Workload
	Clone() Workload
}

// IterationLimiter caps repeated execution for workloads where additional
// samples would not provide independent measurements. Non-positive values mean
// that the configured iteration count is not limited.
type IterationLimiter interface {
	MaxIterations() int
}

// ThroughputMeter can report a workload-specific throughput unit.
type ThroughputMeter interface {
	Throughput(processed int64, elapsed time.Duration) (float64, string)
}

// LatencyWorkload can derive a latency metric from the processed item count.
type LatencyWorkload interface {
	AverageLatencyNS(processed int64, elapsed time.Duration) float64
}

// DetailReporter can return a human-readable detail string for display in reports.
type DetailReporter interface {
	Detail() string
}

// WarmupSkipper marks workloads where a synthetic warm-up pass is undesirable.
// External benchmark tools already perform their own setup and should be
// executed exactly as requested so the reported command/runtime is transparent.
type WarmupSkipper interface {
	SkipWarmup() bool
}
