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
	section.Results = results
	okCount := 0
	for _, item := range results {
		if strings.TrimSpace(item.Error) == "" {
			okCount++
		}
	}
	if len(results) == 0 {
		section.Status = "error"
		section.Message = "no traceroute targets selected"
		return
	}
	if okCount == 0 {
		section.Status = "error"
		section.Message = fmt.Sprintf("0/%d traces ok", len(results))
		return
	}
	section.Status = statusFromCounts(okCount, len(results)-okCount)
	section.Message = fmt.Sprintf("%d/%d traces ok", okCount, len(results))
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
