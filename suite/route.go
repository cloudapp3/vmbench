package suite

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cloudapp3/vmbench/bench/netio"
	"github.com/cloudapp3/vmbench/nodecatalog"
)

func runRouteSection(ctx context.Context, opts Options, report *SuiteReport) {
	section := &report.Route
	section.Status = "running"
	section.StartedTime = time.Now().Unix()
	section.IPVersion = opts.IPVersion
	manifest, ok := catalogManifestForOptions(opts)
	if !ok {
		section.FinishTime = time.Now().Unix()
		section.Status = "error"
		section.Message = "resolved node catalog is required"
		return
	}
	targets := routeTargetsForManifest(manifest, section.RoutePresets, opts.IPVersion)
	results, err := netio.ProbeTracerouteTargets(ctx, targets)
	section.FinishTime = time.Now().Unix()
	if err != nil {
		section.Status = "error"
		section.Message = err.Error()
		return
	}
	// Best-effort line classification from the collected hop evidence; a
	// classification miss never fails the route section.
	netio.ClassifyTraceResults(ctx, results)
	section.Results = results
	section.Status, section.Message = summarizeRouteResults(results)
}

func summarizeRouteResults(results []netio.TraceProbeResult) (string, string) {
	if len(results) == 0 {
		return "error", "no traceroute targets selected"
	}
	reachedCount := 0
	partialCount := 0
	errorCount := 0
	for _, item := range results {
		switch item.EffectiveStatus() {
		case netio.TraceStatusOK:
			reachedCount++
		case netio.TraceStatusPartial:
			partialCount++
		default:
			errorCount++
		}
	}
	status := "partial"
	switch {
	case reachedCount == len(results):
		status = "ok"
	case errorCount == len(results):
		status = "error"
	}
	message := fmt.Sprintf("%d/%d destinations reached", reachedCount, len(results))
	if partialCount > 0 {
		message += fmt.Sprintf("; %d partial", partialCount)
	}
	if errorCount > 0 {
		message += fmt.Sprintf("; %d errors", errorCount)
	}
	return status, message
}

func traceDestinationReachedText(value *bool) string {
	if value == nil {
		return "unknown"
	}
	if *value {
		return "yes"
	}
	return "no"
}

// traceClassificationText renders the optional line classification label.
func traceClassificationText(classification *netio.RouteClassification) string {
	if classification == nil {
		return "-"
	}
	label := strings.TrimSpace(classification.Label)
	if label == "" {
		return defaultText(classification.Code, "-")
	}
	return label
}

func routeTargetsForPresets(presets []string) []netio.TraceTarget {
	manifest, err := nodecatalog.Embedded()
	if err != nil {
		return nil
	}
	return routeTargetsForManifest(manifest, presets, "v4")
}

func routeTargetsForManifest(manifest nodecatalog.Manifest, presets []string, ipVersion string) []netio.TraceTarget {
	targets := netio.TraceTargetsFromManifest(manifest, ipVersion)
	if len(presets) == 0 {
		return targets
	}
	cities := make(map[string]struct{}, len(presets))
	carriers := make(map[string]struct{}, len(presets))
	for _, preset := range presets {
		switch strings.ToLower(strings.TrimSpace(preset)) {
		case "gz":
			cities["guangzhou"] = struct{}{}
		case "bj":
			cities["beijing"] = struct{}{}
		case "sh":
			cities["shanghai"] = struct{}{}
		case "cd":
			cities["chengdu"] = struct{}{}
		case "cernet":
			carriers["cernet"] = struct{}{}
		case "cstnet":
			carriers["cstnet"] = struct{}{}
		}
	}
	out := make([]netio.TraceTarget, 0)
	for _, target := range targets {
		_, cityMatch := cities[strings.ToLower(strings.TrimSpace(target.City))]
		_, carrierMatch := carriers[strings.ToLower(strings.TrimSpace(target.Carrier))]
		if cityMatch || carrierMatch {
			out = append(out, target)
		}
	}
	return out
}
