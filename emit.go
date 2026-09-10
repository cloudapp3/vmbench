package vmbench

import (
	"fmt"
	"strings"
	"time"

	gbbench "github.com/cloudapp3/vmbench/bench"
)

func emitCompletionEvents(opts Options, result gbbench.BenchResult) {
	emitRunDetailEvent(opts, result, result.Result)
}

func emitRunDetailEvent(opts Options, result gbbench.BenchResult, detail *gbbench.RunDetail) {
	if detail == nil {
		return
	}
	kind := EventCheckupDone
	var eventErr error
	message := metricText(detail)
	if strings.TrimSpace(detail.Error) != "" {
		kind = EventCheckupFail
		eventErr = errString(detail.Error)
		message = detail.Error
	}
	emitEvent(opts, Event{
		Kind:      kind,
		Checkup:   workloadKey(result.Workload),
		Workload:  strings.TrimSpace(result.Workload),
		Category:  strings.TrimSpace(result.Category),
		Iteration: detail.Iterations,
		Progress:  1,
		Metric:    metricText(detail),
		Duration:  detail.MedianTime,
		Err:       eventErr,
		Message:   strings.TrimSpace(message),
		Status:    statusForDetail(detail),
	})
}

func workloadKey(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		trimmed = "unknown"
	}
	return trimmed
}

func statusForDetail(detail *gbbench.RunDetail) string {
	if detail == nil {
		return "skip"
	}
	if strings.TrimSpace(detail.Error) != "" {
		return "fail"
	}
	return "done"
}

func metricText(detail *gbbench.RunDetail) string {
	if detail == nil {
		return ""
	}
	if detail.AverageLatencyNS > 0 {
		metric := fmt.Sprintf("%.2f ns/op", detail.AverageLatencyNS)
		if detail.LatencyP99NS > 0 {
			metric += fmt.Sprintf(" (p99 %s)", formatLatencyP99NS(detail.LatencyP99NS))
		}
		return metric
	}
	if detail.Throughput > 0 {
		unit := strings.TrimSpace(detail.ThroughputUnit)
		if unit == "" {
			unit = "ops/s"
		}
		return fmt.Sprintf("%.2f %s", detail.Throughput, unit)
	}
	return detail.MedianTime.Round(time.Millisecond).String()
}

// formatLatencyP99NS renders a nanosecond latency with a human-readable unit.
func formatLatencyP99NS(ns float64) string {
	switch {
	case ns >= 1e6:
		return fmt.Sprintf("%.2f ms", ns/1e6)
	case ns >= 1e3:
		return fmt.Sprintf("%.2f µs", ns/1e3)
	default:
		return fmt.Sprintf("%.2f ns", ns)
	}
}
