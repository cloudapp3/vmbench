package checkup

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cloudapp3/vmbench/bench/netio"
	"github.com/cloudapp3/vmbench/i18n"
	"github.com/cloudapp3/vmbench/nodecatalog"
	"github.com/oneclickvirt/backtrace/model"
)

// routeConfidenceInconclusive mirrors the stable machine-readable confidence
// token of the upstream backtrace classification (its constants are
// unexported). Inconclusive evidence earns no display label, so callers fall
// back to observed carrier ASN evidence instead of showing a dead end.
const routeConfidenceInconclusive = "inconclusive"

// routeASNGrades grades the carrier backbone keys behind the ECS-style line
// labels for display tone, mirroring the upstream rank semantics: 3 marks
// premium/quality backbones (CN2GIA/GT, CTGNET, CMIN2, 9929, CUG), 2 marks
// plain backbone lines (163, 4837, CMI/CMNET).
var routeASNGrades = map[string]int{
	"AS4809a": 3, "AS23764": 3, "AS58807": 3,
	"AS4809b": 3, "AS9929": 3, "AS10099": 3,
	"AS4134": 2, "AS4837": 2, "AS9808": 2, "AS58453": 2,
}

func runRouteSection(ctx context.Context, opts Options, report *CheckupReport) {
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

// RouteLineText renders the ECS-style return-route line label for one trace
// result. The conservative classification label wins (legacy alignment
// whitespace collapsed); reports that predate classification, or that
// resolved inconclusively, fall back to translating observed carrier ASNs
// through the upstream label table; when neither exists a localized
// unknown-line label is returned so line columns never render empty.
func RouteLineText(result netio.TraceProbeResult) string {
	if label := classificationLineLabel(result.Classification); label != "" {
		return label
	}
	if labels := observedLineLabels(result.ObservedASNs); len(labels) > 0 {
		return strings.Join(labels, " ")
	}
	return i18n.T("report.checkup.route.unknownLine")
}

// RouteLineTone grades the return-route evidence for display color, shared
// by console, HTML, and TUI so every face agrees: "ok" marks premium or
// quality lines (upstream rank >= 3), "muted" marks confirmed plain backbone
// lines such as 163/4837/CMI, and "warn" marks inconclusive or missing
// evidence.
func RouteLineTone(result netio.TraceProbeResult) string {
	if label := classificationLineLabel(result.Classification); label != "" {
		switch {
		case result.Classification.Rank >= 3:
			return "ok"
		case result.Classification.Rank == 2:
			return "muted"
		default:
			return "warn"
		}
	}
	best := 0
	for _, key := range observedLineKeys(result.ObservedASNs) {
		if grade := routeASNGrades[key]; grade > best {
			best = grade
		}
	}
	switch {
	case best >= 3:
		return "ok"
	case best == 2:
		return "muted"
	default:
		return "warn"
	}
}

// classificationLineLabel renders the classification display label. Non-empty
// labels fall back to the machine code; inconclusive evidence yields nothing
// so callers can descend to observed ASN evidence.
func classificationLineLabel(classification *netio.RouteClassification) string {
	if classification == nil || classification.Confidence == routeConfidenceInconclusive {
		return ""
	}
	if label := strings.Join(strings.Fields(classification.Label), " "); label != "" {
		return label
	}
	return strings.TrimSpace(classification.Code)
}

// observedLineLabels translates the ordered carrier backbone keys into the
// classic ECS line labels from the upstream table.
func observedLineLabels(asns []string) []string {
	keys := observedLineKeys(asns)
	labels := make([]string, 0, len(keys))
	seen := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		label := strings.Join(strings.Fields(model.M[key]), " ")
		if label == "" {
			continue
		}
		if _, exists := seen[label]; exists {
			continue
		}
		seen[label] = struct{}{}
		labels = append(labels, label)
	}
	return labels
}

// observedLineKeys normalizes and orders observed China-carrier backbone
// ASNs, mirroring the upstream renderer's AS4809 disambiguation: AS4809 seen
// together with AS4134 is the CN2 GT hand-off toward the 163 backbone, while
// AS4809 alone indicates the CN2 GIA backbone. The raw AS4809 key is
// superseded by that pair and dropped.
func observedLineKeys(asns []string) []string {
	seen := make(map[string]struct{}, len(asns))
	normalized := make([]string, 0, len(asns))
	for _, asn := range asns {
		asn = strings.ToUpper(strings.TrimSpace(asn))
		if asn == "" {
			continue
		}
		if _, exists := seen[asn]; exists {
			continue
		}
		seen[asn] = struct{}{}
		normalized = append(normalized, asn)
	}
	if len(normalized) == 0 {
		return nil
	}
	ordered := make([]string, 0, len(normalized)+1)
	if _, has4809 := seen["AS4809"]; has4809 {
		if _, has4134 := seen["AS4134"]; has4134 {
			ordered = append(ordered, "AS4809b")
		} else {
			ordered = append(ordered, "AS4809a")
		}
	}
	for _, asn := range normalized {
		if asn == "AS4809" {
			continue
		}
		ordered = append(ordered, asn)
	}
	return ordered
}

// routeConfidenceText renders the raw classification confidence token. Raw
// tokens stay untranslated like the ok/partial/error status vocabulary; a
// missing or inconclusive classification renders as a dash.
func routeConfidenceText(classification *netio.RouteClassification) string {
	if classification == nil || classification.Confidence == routeConfidenceInconclusive {
		return "-"
	}
	return classification.Confidence
}

// routeStatusText merges reachability status with destination evidence; the
// per-target hop list and probe provenance remain available in HTML and JSON.
func routeStatusText(item RouteRun) string {
	status := item.EffectiveStatus()
	if item.DestinationReached != nil && !*item.DestinationReached && status != netio.TraceStatusError {
		status += " (unreached)"
	}
	if message := strings.TrimSpace(item.Error); message != "" {
		status += ": " + message
	}
	return status
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
