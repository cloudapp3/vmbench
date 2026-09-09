package vmbench

import (
	"context"
	"strings"

	gbbench "github.com/cloudapp3/vmbench/bench"
	gbreport "github.com/cloudapp3/vmbench/report"
	"github.com/cloudapp3/vmbench/sysinfo"
)

// RunCore runs the selected vmbench workloads and returns the report document.
func RunCore(ctx context.Context, opts Options) Report {
	prepared, filterExpr, warnings := prepareOptions(opts)
	norm, configErr := NormalizeOptions(opts)
	if configErr != nil {
		norm = prepared
		warnings = append(warnings, "configuration: "+configErr.Error())
	}
	system, sysWarnings := sysinfo.Collect(ctx)
	warnings = append(warnings, sysWarnings...)

	var workloads []gbbench.Workload
	if configErr == nil {
		workloads = buildWorkloads(norm.DiskPath, filterExpr, norm.HardwareTools)
	}
	if len(workloads) == 0 {
		warnings = append(warnings, "no workloads matched the current filter")
	}

	results, runWarnings := gbbench.RunAll(ctx, workloads, gbbench.RunConfig{
		Iterations: norm.Iterations,
		Timeout:    norm.Timeout,
		OnWorkloadStart: func(progress gbbench.ProgressEvent) {
			emitEvent(norm, Event{
				Kind:      EventCheckupStart,
				Checkup:   workloadKey(progress.Workload),
				Workload:  strings.TrimSpace(progress.Workload),
				Iteration: 0,
				Current:   progress.Current,
				Total:     progress.Total,
				Message:   "started",
			})
		},
		OnProgress: func(progress gbbench.ProgressEvent) {
			progressValue := 0.0
			if progress.Total > 0 {
				progressValue = float64(progress.Current) / float64(progress.Total)
			}
			emitEvent(norm, Event{
				Kind:      EventCheckupProgress,
				Checkup:   workloadKey(progress.Workload),
				Workload:  strings.TrimSpace(progress.Workload),
				Iteration: progress.Iteration,
				Current:   progress.Current,
				Total:     progress.Total,
				Progress:  clamp01(progressValue),
				Message:   strings.TrimSpace(progress.Status),
				Status:    strings.TrimSpace(progress.Status),
			})
		},
		OnWorkloadDone: func(result gbbench.BenchResult) {
			emitCompletionEvents(norm, result)
		},
	})
	warnings = append(warnings, runWarnings...)

	document := gbreport.BuildDocument(Version, system, gbreport.RunConfig{
		Iterations:    norm.Iterations,
		Filter:        filterExpr,
		DiskPath:      norm.DiskPath,
		Scope:         ScopeHardware,
		HardwareTools: append([]string(nil), norm.HardwareTools...),
	}, results, warnings)

	emitEvent(norm, Event{
		Kind:     EventBenchDone,
		Progress: 1,
	})
	return document
}
