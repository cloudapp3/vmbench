package vmbench

import (
	"context"
	"strings"

	gbbench "github.com/cloudapp3/vmbench/bench"
	"github.com/cloudapp3/vmbench/nodecatalog"
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
	if norm.CatalogWarning != "" {
		warnings = append(warnings, norm.CatalogWarning)
	}
	system, sysWarnings := sysinfo.Collect(ctx)
	warnings = append(warnings, sysWarnings...)

	var workloads []gbbench.Workload
	if configErr == nil {
		workloads = buildWorkloads(norm.DiskPath, filterExpr, norm.Engine, norm.Scope, norm.IperfHosts, norm.HardwareTools, norm.ResolvedCatalog)
	}
	if len(workloads) == 0 {
		warnings = append(warnings, "no workloads matched the current filter")
	}

	results, runWarnings := gbbench.RunAll(ctx, workloads, gbbench.RunConfig{
		Iterations: norm.Iterations,
		Timeout:    norm.Timeout,
		OnWorkloadStart: func(progress gbbench.ProgressEvent) {
			emitEvent(norm, Event{
				Kind:      EventSuiteStart,
				Suite:     workloadKey(progress.Workload),
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
				Kind:      EventSuiteProgress,
				Suite:     workloadKey(progress.Workload),
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
		Iterations:      norm.Iterations,
		Filter:          filterExpr,
		DiskPath:        norm.DiskPath,
		Extensions:      norm.Scope != ScopeHardware,
		Mode:            norm.Mode,
		Scope:           norm.Scope,
		HardwareTools:   append([]string(nil), norm.HardwareTools...),
		IperfHosts:      append([]string(nil), norm.IperfHosts...),
		CatalogSource:   norm.CatalogSource,
		CatalogRevision: norm.CatalogRevision,
		NodeIDs:         catalogNodeIDs(norm, workloads),
	}, results, warnings)

	emitEvent(norm, Event{
		Kind:     EventBenchDone,
		Progress: 1,
	})
	return document
}

func catalogNodeIDs(opts Options, workloads []gbbench.Workload) []string {
	if opts.ResolvedCatalog == nil {
		return nil
	}
	selectedWorkloads := make(map[string]struct{}, len(workloads))
	for _, workload := range workloads {
		selectedWorkloads[workload.Name()] = struct{}{}
	}
	selected := make(map[string]struct{})
	for _, node := range opts.ResolvedCatalog.Select(nodecatalog.Filter{Kind: nodecatalog.KindDownload}) {
		if _, ok := selectedWorkloads["Net Download ("+node.Name+")"]; ok {
			selected[node.ID] = struct{}{}
		}
	}
	_, usesPing := selectedWorkloads["Net Ping"]
	_, usesRoute := selectedWorkloads["Net Traceroute"]
	if usesPing {
		for _, id := range opts.ResolvedCatalog.NodeIDs(nodecatalog.Filter{Kind: nodecatalog.KindPing, IPFamily: "v4"}) {
			selected[id] = struct{}{}
		}
	}
	if usesRoute {
		for _, id := range opts.ResolvedCatalog.NodeIDs(nodecatalog.Filter{Kind: nodecatalog.KindRoute, IPFamily: "v4"}) {
			selected[id] = struct{}{}
		}
	}
	ids := make([]string, 0, len(selected))
	for _, node := range opts.ResolvedCatalog.Nodes {
		if _, ok := selected[node.ID]; ok {
			ids = append(ids, node.ID)
		}
	}
	return ids
}
