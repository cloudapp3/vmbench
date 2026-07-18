package bench

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"github.com/cloudapp3/vmbench/bench/common"
)

// RunDetail stores the measured details for one workload run.
type RunDetail struct {
	Iterations       int             `json:"iterations"`
	MedianTime       time.Duration   `json:"median_time"`
	Throughput       float64         `json:"throughput"`
	ThroughputUnit   string          `json:"throughput_unit"`
	Samples          []time.Duration `json:"samples,omitempty"`
	BytesProcessed   int64           `json:"bytes_processed,omitempty"`
	OpsProcessed     float64         `json:"ops_processed,omitempty"`
	AverageLatencyNS float64         `json:"average_latency_ns,omitempty"`
	Detail           string          `json:"detail,omitempty"`
	Error            string          `json:"error,omitempty"`
}

// BenchResult stores the measured result for one workload.
type BenchResult struct {
	Workload    string     `json:"workload"`
	Category    string     `json:"category"`
	Description string     `json:"description"`
	Result      *RunDetail `json:"result,omitempty"`
}

// ProgressEvent reports one unit of progress to callers.
type ProgressEvent struct {
	Current   int
	Total     int
	Workload  string
	Iteration int
	Status    string
}

// RunConfig controls how workloads are executed.
type RunConfig struct {
	Iterations      int
	Filter          *regexp.Regexp
	Timeout         time.Duration
	OnWorkloadStart func(ProgressEvent)
	OnProgress      func(ProgressEvent)
	OnWorkloadDone  func(BenchResult)
	MultiCore       bool // retained for compatibility; workloads are always isolated serially
}

// RunAll executes workloads according to config and returns their measured results.
func RunAll(ctx context.Context, workloads []Workload, config RunConfig) ([]BenchResult, []string) {
	filtered := filterWorkloads(workloads, config.Filter)
	return runAllSequential(ctx, filtered, config)
}

func runAllSequential(ctx context.Context, workloads []Workload, config RunConfig) ([]BenchResult, []string) {
	iterations := config.Iterations
	if iterations <= 0 {
		iterations = 3
	}
	total := plannedSamples(workloads, iterations)
	current := 0
	warnings := make([]string, 0)
	results := make([]BenchResult, 0, len(workloads))
	for _, workload := range workloads {
		entry := BenchResult{
			Workload:    workload.Name(),
			Category:    workload.Category(),
			Description: workload.Description(),
		}
		actualIterations := workloadIterations(workload, iterations)
		emitProgress(config.OnWorkloadStart, current, total, workload.Name(), 0, "started")
		detail, err := runWorkload(ctx, workload, actualIterations, config.Timeout, func(iter int, status string) {
			current++
			emitProgress(config.OnProgress, current, total, workload.Name(), iter, status)
		})
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: %v", workload.Name(), err))
		}
		entry.Result = detail
		results = append(results, entry)
		if config.OnWorkloadDone != nil {
			config.OnWorkloadDone(entry)
		}
	}
	return results, warnings
}

func filterWorkloads(workloads []Workload, filter *regexp.Regexp) []Workload {
	if filter == nil {
		return append([]Workload(nil), workloads...)
	}
	out := make([]Workload, 0, len(workloads))
	for _, workload := range workloads {
		if filter.MatchString(workload.Name()) || filter.MatchString(workload.Category()) {
			out = append(out, workload)
		}
	}
	return out
}

func plannedSamples(workloads []Workload, iterations int) int {
	total := 0
	for _, workload := range workloads {
		total += workloadIterations(workload, iterations)
	}
	return total
}

func workloadIterations(workload Workload, iterations int) int {
	iterations = maxInt(iterations, 1)
	limiter, ok := workload.(IterationLimiter)
	if !ok {
		return iterations
	}
	limit := limiter.MaxIterations()
	if limit > 0 && limit < iterations {
		return limit
	}
	return iterations
}

func emitProgress(fn func(ProgressEvent), current, total int, workload string, iteration int, status string) {
	if fn == nil {
		return
	}
	fn(ProgressEvent{Current: current, Total: total, Workload: workload, Iteration: iteration, Status: status})
}

func runWorkload(ctx context.Context, workload Workload, iterations int, timeout time.Duration, after func(iter int, status string)) (*RunDetail, error) {
	iterations = workloadIterations(workload, iterations)
	samples := make([]time.Duration, 0, iterations)
	processedSamples := make([]int64, 0, iterations)
	throughputSamples := make([]float64, 0, iterations)
	latencySamples := make([]float64, 0, iterations)
	throughputUnit := ""
	processedKind := ProcessedUnknown
	processedKindConsistent := true
	var lastErr error
	for iteration := 1; iteration <= iterations; iteration++ {
		result := runSingleSample(ctx, workload, timeout)
		if result.err != nil {
			lastErr = result.err
			after(iteration, "error")
			continue
		}
		if len(samples) == 0 {
			processedKind = result.processedKind
		} else if result.processedKind != processedKind {
			processedKindConsistent = false
		}
		samples = append(samples, result.elapsed)
		processedSamples = append(processedSamples, result.processed)
		if result.throughput > 0 {
			throughputSamples = append(throughputSamples, result.throughput)
			if result.throughputUnit != "" {
				throughputUnit = result.throughputUnit
			}
		}
		if result.averageLatencyNS > 0 {
			latencySamples = append(latencySamples, result.averageLatencyNS)
		}
		after(iteration, "ok")
	}
	if len(samples) == 0 {
		return &RunDetail{Iterations: iterations, Error: errorText(lastErr)}, lastErr
	}
	medianTime := common.MedianDuration(samples)
	medianProcessed := common.MedianInt64(processedSamples)
	detail := &RunDetail{
		Iterations: iterations,
		MedianTime: medianTime,
		Samples:    samples,
	}
	if processedKindConsistent {
		switch processedKind {
		case ProcessedBytes:
			detail.BytesProcessed = medianProcessed
		case ProcessedOperations:
			detail.OpsProcessed = float64(medianProcessed)
		}
	}
	if len(throughputSamples) > 0 {
		detail.Throughput = common.MedianFloat64(throughputSamples)
		detail.ThroughputUnit = throughputUnit
	} else if meter, ok := workload.(ThroughputMeter); ok {
		detail.Throughput, detail.ThroughputUnit = meter.Throughput(medianProcessed, medianTime)
	} else {
		detail.Throughput = common.MBPerSecond(medianProcessed, medianTime)
		detail.ThroughputUnit = "MiB/s"
	}
	if len(latencySamples) > 0 {
		detail.AverageLatencyNS = common.MedianFloat64(latencySamples)
	} else if latency, ok := workload.(LatencyWorkload); ok {
		detail.AverageLatencyNS = latency.AverageLatencyNS(medianProcessed, medianTime)
	}
	if reporter, ok := workload.(DetailReporter); ok {
		detail.Detail = reporter.Detail()
	}
	if lastErr != nil {
		detail.Error = errorText(lastErr)
	}
	return detail, lastErr
}

func runSingleSample(ctx context.Context, workload Workload, timeout time.Duration) sampleResult {
	instance := cloneWorkload(workload)
	return runSampleWithWarmup(ctx, instance, timeout)
}

type sampleResult struct {
	elapsed          time.Duration
	processed        int64
	processedKind    ProcessedKind
	throughput       float64
	throughputUnit   string
	averageLatencyNS float64
	err              error
}

func runSampleWithWarmup(ctx context.Context, workload Workload, timeout time.Duration) sampleResult {
	child, cancel := context.WithTimeout(ctx, prepareTimeout(timeout))
	defer cancel()
	if skipper, ok := workload.(WarmupSkipper); ok && skipper.SkipWarmup() {
		return runMeasuredOnly(child, workload)
	}
	_, _, warmErr := workload.Run(child)
	if warmErr != nil {
		return sampleResult{err: warmErr}
	}
	if err := workload.Validate(); err != nil {
		return sampleResult{err: err}
	}
	return runMeasuredOnly(child, workload)
}

func runMeasuredOnly(ctx context.Context, workload Workload) sampleResult {
	result := sampleResult{}
	result.elapsed, result.processed, result.err = workload.Run(ctx)
	if result.err != nil {
		return result
	}
	if err := workload.Validate(); err != nil {
		result.err = err
	}
	if reporter, ok := workload.(ProcessedMetricReporter); ok {
		result.processedKind = reporter.ProcessedKind()
	}
	result.captureMetrics(workload)
	return result
}

func (r *sampleResult) captureMetrics(workload Workload) {
	if r == nil || r.err != nil {
		return
	}
	if meter, ok := workload.(ThroughputMeter); ok {
		r.throughput, r.throughputUnit = meter.Throughput(r.processed, r.elapsed)
	} else {
		r.throughput = common.MBPerSecond(r.processed, r.elapsed)
		r.throughputUnit = "MiB/s"
	}
	if latency, ok := workload.(LatencyWorkload); ok {
		r.averageLatencyNS = latency.AverageLatencyNS(r.processed, r.elapsed)
	}
}

func prepareTimeout(timeout time.Duration) time.Duration {
	if timeout <= 0 {
		return 5 * time.Minute
	}
	return timeout
}

func cloneWorkload(workload Workload) Workload {
	if cloneable, ok := workload.(CloneableWorkload); ok {
		return cloneable.Clone()
	}
	return workload
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
