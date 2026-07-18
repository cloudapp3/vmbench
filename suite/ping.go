package suite

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cloudapp3/vmbench/bench/netio"
	"github.com/cloudapp3/vmbench/nodecatalog"
)

func runPingSection(ctx context.Context, opts Options, report *SuiteReport) {
	section := &report.Ping
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
	targets := pingTargetsForManifest(manifest, opts.RoutePresets, opts.IPVersion)
	results, err := netio.ProbePingTargets(ctx, targets)
	section.FinishTime = time.Now().Unix()
	section.Results = results
	if err != nil {
		section.Status = "error"
		section.Message = err.Error()
		return
	}
	okCount := 0
	var avgSum float64
	for _, item := range results {
		if item.Status == "ok" {
			okCount++
			avgSum += item.AvgLatencyMs
		}
	}
	if len(results) == 0 {
		section.Status = "error"
		section.Message = "no ping targets selected"
		return
	}
	if okCount == 0 {
		section.Status = "error"
		section.Message = fmt.Sprintf("0/%d pings ok", len(results))
		return
	}
	section.Status = statusFromCounts(okCount, len(results)-okCount)
	section.Message = fmt.Sprintf("%d/%d pings ok · avg %.1f ms", okCount, len(results), avgSum/float64(okCount))
}

func pingTargetsForPresets(presets []string, ipVersion string) []netio.PingTarget {
	manifest, err := nodecatalog.Embedded()
	if err != nil {
		return nil
	}
	return pingTargetsForManifest(manifest, presets, ipVersion)
}

func pingTargetsForManifest(manifest nodecatalog.Manifest, presets []string, ipVersion string) []netio.PingTarget {
	targets := netio.PingTargetsFromManifest(manifest, ipVersion)
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
	out := make([]netio.PingTarget, 0, len(targets))
	for _, target := range targets {
		_, cityMatch := cities[strings.ToLower(strings.TrimSpace(target.City))]
		_, carrierMatch := carriers[strings.ToLower(strings.TrimSpace(target.Carrier))]
		if cityMatch || carrierMatch {
			out = append(out, target)
		}
	}
	return out
}
