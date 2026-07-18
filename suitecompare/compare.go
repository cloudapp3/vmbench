// Package suitecompare compares raw suite reports without discarding unknown sections.
package suitecompare

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"
)

// Header identifies one report column.
type Header struct {
	ReportID        string
	Label           string
	StartedAt       time.Time
	CatalogRevision string
}

// MetricValue is one report's value and comparability context.
type MetricValue struct {
	Available       bool
	Value           float64
	Unit            string
	Protocol        string
	Provider        string
	Target          string
	CatalogRevision string
}

// MetricComparison is one raw metric aligned across suite reports.
type MetricComparison struct {
	Section    string
	Name       string
	Values     []MetricValue
	Delta      string
	Comparable bool
	Reason     string
}

// Result is a structured suite comparison.
type Result struct {
	Reports  []Header
	Metrics  []MetricComparison
	Warnings []string
}

type direction uint8

const (
	directionNeutral direction = iota
	directionLower
	directionHigher
)

type measurement struct {
	section         string
	name            string
	value           float64
	unit            string
	protocol        string
	provider        string
	target          string
	catalogRevision string
	requireCatalog  bool
	direction       direction
}

type decodedReport struct {
	header       Header
	measurements map[string]measurement
}

type metricContext struct {
	protocol string
	provider string
	target   string
}

// Compare parses and aligns two or more suite JSON reports.
func Compare(reports [][]byte) (Result, error) {
	if len(reports) < 2 {
		return Result{}, errors.New("at least 2 suite reports required for comparison")
	}
	decoded := make([]decodedReport, len(reports))
	for i, raw := range reports {
		item, err := decodeReport(raw, i)
		if err != nil {
			return Result{}, fmt.Errorf("report %d: %w", i+1, err)
		}
		decoded[i] = item
	}

	result := Result{Reports: make([]Header, len(decoded))}
	keys := make(map[string]struct{})
	for i, item := range decoded {
		result.Reports[i] = item.header
		for key := range item.measurements {
			keys[key] = struct{}{}
		}
	}
	sortedKeys := make([]string, 0, len(keys))
	for key := range keys {
		sortedKeys = append(sortedKeys, key)
	}
	sort.Strings(sortedKeys)

	seenWarnings := make(map[string]struct{})
	for _, key := range sortedKeys {
		comparison := alignMeasurement(key, decoded)
		result.Metrics = append(result.Metrics, comparison)
		if comparison.Reason == "" {
			continue
		}
		warning := fmt.Sprintf("%s/%s: %s", comparison.Section, comparison.Name, comparison.Reason)
		if _, seen := seenWarnings[warning]; seen {
			continue
		}
		seenWarnings[warning] = struct{}{}
		result.Warnings = append(result.Warnings, warning)
	}
	return result, nil
}

// WriteCompare renders a side-by-side suite comparison.
func WriteCompare(w io.Writer, reports [][]byte) error {
	if w == nil {
		return errors.New("comparison writer is nil")
	}
	result, err := Compare(reports)
	if err != nil {
		return err
	}

	line := strings.Repeat("═", 72)
	fmt.Fprintf(w, "%s\n  VMBench Suite Compare\n%s\n\n", line, line)
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "Property\t"+joinHeaders(result.Reports))
	printHeaderRow(tw, "Report ID", result.Reports, func(header Header) string {
		if header.ReportID == "" {
			return "legacy/unknown"
		}
		return header.ReportID
	})
	printHeaderRow(tw, "Catalog", result.Reports, func(header Header) string {
		if strings.TrimSpace(header.CatalogRevision) == "" {
			return "unknown"
		}
		return header.CatalogRevision
	})
	_ = tw.Flush()

	if len(result.Warnings) > 0 {
		fmt.Fprintln(w, "\nComparability warnings:")
		for _, warning := range result.Warnings {
			fmt.Fprintf(w, "  - %s\n", warning)
		}
	}

	fmt.Fprintf(w, "\n%s\n  Raw Metrics\n%s\n", line, line)
	tw = tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "Section\tMetric\t"+joinHeaders(result.Reports)+"\tDelta")
	for _, metric := range result.Metrics {
		row := metric.Section + "\t" + metric.Name + "\t"
		for _, value := range metric.Values {
			if !value.Available {
				row += "-\t"
				continue
			}
			row += formatMeasured(value.Value, value.Unit) + "\t"
		}
		row += metric.Delta
		fmt.Fprintln(tw, row)
	}
	_ = tw.Flush()
	_, _ = fmt.Fprintln(w)
	return nil
}

func decodeReport(data []byte, index int) (decodedReport, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var root map[string]any
	if err := decoder.Decode(&root); err != nil {
		return decodedReport{}, fmt.Errorf("invalid JSON: %w", err)
	}
	if root == nil {
		return decodedReport{}, errors.New("suite report must be a JSON object")
	}
	if kind := stringValue(root["report_kind"]); kind != "" && !strings.EqualFold(kind, "suite") {
		return decodedReport{}, fmt.Errorf("report_kind is %q, want suite", kind)
	}
	if !looksLikeSuite(root) {
		return decodedReport{}, errors.New("JSON is not a recognized suite report")
	}

	revision := catalogRevision(root)
	header := Header{
		ReportID:        stringValue(root["report_id"]),
		StartedAt:       reportTime(root),
		CatalogRevision: revision,
	}
	header.Label = reportLabel(root, index)
	out := decodedReport{header: header, measurements: make(map[string]measurement)}

	knownSections := []string{"hardware", "route", "ping", "speed", "ip_quality", "mail", "media"}
	for _, section := range knownSections {
		value, ok := asMap(root[section])
		if !ok {
			continue
		}
		extractKnownSection(out.measurements, section, value, revision)
	}
	if sections, ok := asMap(root["sections"]); ok {
		for name, raw := range sections {
			section, ok := asMap(raw)
			if !ok {
				continue
			}
			extractGenericSection(out.measurements, name, section, revision)
		}
	}
	for name, raw := range root {
		if isEnvelopeKey(name) || contains(knownSections, name) || name == "sections" {
			continue
		}
		section, ok := asMap(raw)
		if !ok || !looksLikeSection(section) {
			continue
		}
		extractGenericSection(out.measurements, name, section, revision)
	}
	return out, nil
}

func extractKnownSection(out map[string]measurement, name string, section map[string]any, revision string) {
	if enabled, exists := boolValue(section["enabled"]); exists && !enabled {
		return
	}
	switch name {
	case "hardware":
		extractHardware(out, section)
	case "route":
		extractRoute(out, section, revision)
	case "ping":
		extractPing(out, section, revision)
	case "speed":
		extractSpeed(out, section, revision)
	default:
		extractGenericSection(out, name, section, revision)
	}
}

func extractHardware(out map[string]measurement, section map[string]any) {
	report, ok := asMap(section["report"])
	if !ok {
		return
	}
	for _, groupName := range []string{"results", "extensions"} {
		group, ok := asMap(report[groupName])
		if !ok {
			continue
		}
		workloads, _ := group["workloads"].([]any)
		for index, raw := range workloads {
			workload, ok := asMap(raw)
			if !ok {
				continue
			}
			name := firstString(workload, "name", "id")
			if name == "" {
				name = strconv.Itoa(index + 1)
			}
			result, ok := asMap(workload["result"])
			if !ok || strings.TrimSpace(stringValue(result["error"])) != "" {
				continue
			}
			provider := hardwareProvider(name)
			addNumber(out, measurementKey("hardware", name, "median_ms"), measurement{
				section: "hardware", name: name + "/median", unit: "ms", protocol: "benchmark",
				provider: provider, target: name, direction: directionLower,
			}, result["median_ms"])
			unit := stringValue(result["throughput_unit"])
			if unit == "" {
				unit = "per second"
			}
			addNumber(out, measurementKey("hardware", name, "throughput"), measurement{
				section: "hardware", name: name + "/throughput", unit: unit, protocol: "benchmark",
				provider: provider, target: name, direction: directionFor("throughput", unit),
			}, result["throughput_per_sec"])
			addNumber(out, measurementKey("hardware", name, "latency"), measurement{
				section: "hardware", name: name + "/latency", unit: "ns/op", protocol: "benchmark",
				provider: provider, target: name, direction: directionLower,
			}, result["avg_ns_per_access"])
		}
	}
}

func extractRoute(out map[string]measurement, section map[string]any, revision string) {
	results, _ := section["results"].([]any)
	for index, raw := range results {
		item, ok := asMap(raw)
		if !ok || strings.TrimSpace(stringValue(item["error"])) != "" {
			continue
		}
		target, _ := asMap(item["target"])
		identity := firstString(target, "id", "name", "endpoint")
		if identity == "" {
			identity = strconv.Itoa(index + 1)
		}
		endpoint := endpointWithPort(firstString(target, "endpoint", "id", "name"), target["port"])
		protocol := firstString(item, "probe_protocol")
		legacyProtocol := protocol == ""
		if legacyProtocol {
			protocol = firstString(target, "protocol")
		}
		if strings.EqualFold(protocol, "tcp") {
			protocol = "tcp-traceroute"
		} else if strings.EqualFold(protocol, "udp") {
			protocol = "udp-traceroute"
		} else if strings.EqualFold(protocol, "icmp") {
			protocol = "icmp-traceroute"
		}
		if protocol == "" {
			protocol = "traceroute"
		}
		if family := firstString(target, "ip_family"); family != "" {
			protocol += "/" + strings.ToLower(family)
		}
		provider := firstString(target, "source")
		if provider == "" {
			provider = "node-catalog"
		}
		if tool := firstString(item, "probe_tool"); tool != "" {
			provider += "/" + tool
		} else if legacyProtocol {
			provider += "/legacy-system-traceroute"
		}
		hops, _ := item["hops"].([]any)
		if len(hops) > 0 {
			addMeasurement(out, measurementKey("route", identity, "hop_count"), measurement{
				section: "route", name: identity + "/hop count", value: float64(len(hops)), unit: "hops",
				protocol: protocol, provider: provider, target: endpoint,
				catalogRevision: revision, requireCatalog: true, direction: directionNeutral,
			})
		}
		for hopIndex := len(hops) - 1; hopIndex >= 0; hopIndex-- {
			hop, ok := asMap(hops[hopIndex])
			if !ok {
				continue
			}
			value, ok := numberValue(hop["rtt_ms"])
			if !ok || value <= 0 {
				continue
			}
			addMeasurement(out, measurementKey("route", identity, "last_hop_rtt_ms"), measurement{
				section: "route", name: identity + "/last responding hop latency", value: value, unit: "ms",
				protocol: protocol, provider: provider, target: endpoint,
				catalogRevision: revision, requireCatalog: true, direction: directionLower,
			})
			break
		}
	}
}

func extractPing(out map[string]measurement, section map[string]any, revision string) {
	results, _ := section["results"].([]any)
	for index, raw := range results {
		item, ok := asMap(raw)
		if !ok || strings.EqualFold(stringValue(item["status"]), "error") {
			continue
		}
		identity := firstString(item, "id", "name", "target")
		if identity == "" {
			identity = strconv.Itoa(index + 1)
		}
		target := endpointWithPort(firstString(item, "target", "id", "name"), item["port"])
		family := firstString(item, "ip_family")
		protocol := firstString(item, "probe_protocol")
		legacyProtocol := protocol == ""
		if legacyProtocol {
			protocol = firstString(item, "protocol")
			if protocol == "" {
				protocol = "tcp"
			}
			protocol += "-connect"
		}
		if family != "" {
			protocol += "/" + strings.ToLower(family)
		}
		provider := firstString(item, "source")
		if provider == "" {
			provider = "node-catalog"
		}
		if tool := firstString(item, "probe_tool"); tool != "" {
			provider += "/" + tool
		} else if legacyProtocol {
			provider += "/legacy-go-net-dialer"
		}
		for _, metric := range []struct {
			field     string
			label     string
			unit      string
			direction direction
		}{
			{field: "avg_latency_ms", label: "latency", unit: "ms", direction: directionLower},
			{field: "jitter_ms", label: "jitter", unit: "ms", direction: directionLower},
			{field: "packet_loss", label: "packet loss", unit: "%", direction: directionLower},
		} {
			addNumber(out, measurementKey("ping", identity, metric.field), measurement{
				section: "ping", name: identity + "/" + metric.label, unit: metric.unit,
				protocol: protocol, provider: provider, target: target,
				catalogRevision: revision, requireCatalog: true, direction: metric.direction,
			}, item[metric.field])
		}
	}
}

func extractSpeed(out map[string]measurement, section map[string]any, revision string) {
	result, ok := asMap(section["result"])
	if !ok {
		return
	}
	providers, _ := result["providers"].([]any)
	if len(providers) == 0 {
		if summary, ok := asMap(result["summary"]); ok {
			providers = []any{summary}
		}
	}
	for index, raw := range providers {
		item, ok := asMap(raw)
		if !ok || strings.EqualFold(stringValue(item["status"]), "error") {
			continue
		}
		provider := firstString(item, "provider", "id")
		if provider == "" {
			provider = "unknown"
		}
		identity := firstString(item, "id", "kind")
		if identity == "" {
			identity = provider + "-" + strconv.Itoa(index+1)
		}
		target := speedTargetIdentity(item, provider)
		protocol := speedProtocol(provider)
		for _, metric := range []struct {
			field     string
			label     string
			unit      string
			direction direction
		}{
			{field: "download_mbps", label: "download", unit: "Mbps", direction: directionHigher},
			{field: "upload_mbps", label: "upload", unit: "Mbps", direction: directionHigher},
			{field: "latency_ms", label: "latency", unit: "ms", direction: directionLower},
		} {
			addNumber(out, measurementKey("speed", identity, metric.field), measurement{
				section: "speed", name: identity + "/" + metric.label, unit: metric.unit,
				protocol: protocol, provider: provider, target: target,
				catalogRevision: revision, requireCatalog: true, direction: metric.direction,
			}, item[metric.field])
		}
	}
}

func extractGenericSection(out map[string]measurement, name string, section map[string]any, revision string) {
	if enabled, exists := boolValue(section["enabled"]); exists && !enabled {
		return
	}
	root := any(section)
	if result, ok := section["result"]; ok {
		root = result
	} else if results, ok := section["results"]; ok {
		root = results
	}
	ctx := metricContext{protocol: protocolForSection(name), provider: name}
	walkGeneric(out, name, root, nil, ctx, revision)
}

func walkGeneric(out map[string]measurement, section string, value any, path []string, ctx metricContext, revision string) {
	switch current := value.(type) {
	case map[string]any:
		ctx = updateContext(ctx, current)
		if strings.EqualFold(stringValue(current["status"]), "error") || strings.TrimSpace(stringValue(current["error"])) != "" {
			return
		}
		keys := make([]string, 0, len(current))
		for key := range current {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if skipGenericField(key) {
				continue
			}
			child := current[key]
			if number, ok := numberValue(child); ok {
				unit := genericUnit(key, current)
				identity := append(append([]string(nil), path...), key)
				label := strings.Join(identity, "/")
				if label == "" {
					label = key
				}
				addMeasurement(out, measurementKey(section, strings.Join(identity, "/"), "value"), measurement{
					section: section, name: label, value: number, unit: unit,
					protocol:        firstNonEmpty(ctx.protocol, protocolForSection(section)),
					provider:        firstNonEmpty(ctx.provider, section),
					target:          firstNonEmpty(ctx.target, strings.Join(path, "/"), section),
					catalogRevision: revision, requireCatalog: sectionRequiresCatalog(section),
					direction: directionFor(key, unit),
				})
				continue
			}
			walkGeneric(out, section, child, append(path, key), ctx, revision)
		}
	case []any:
		for index, child := range current {
			identity := arrayIdentity(child, index)
			walkGeneric(out, section, child, append(path, identity), ctx, revision)
		}
	}
}

func alignMeasurement(key string, reports []decodedReport) MetricComparison {
	values := make([]MetricValue, len(reports))
	var template measurement
	found := false
	measurements := make([]measurement, len(reports))
	for i, report := range reports {
		measurement, ok := report.measurements[key]
		if !ok {
			continue
		}
		measurements[i] = measurement
		values[i] = MetricValue{
			Available: true, Value: measurement.value, Unit: measurement.unit,
			Protocol: measurement.protocol, Provider: measurement.provider, Target: measurement.target,
			CatalogRevision: measurement.catalogRevision,
		}
		if !found {
			template = measurement
			found = true
		}
	}
	comparison := MetricComparison{Section: template.section, Name: template.name, Values: values, Delta: "N/A"}
	reasons := comparabilityReasons(measurements, values, template.requireCatalog)
	if len(reasons) > 0 {
		comparison.Reason = strings.Join(reasons, "; ")
		return comparison
	}
	comparison.Comparable = true
	comparison.Delta = formatDelta(values[0].Value, values[len(values)-1].Value, template.direction)
	return comparison
}

func comparabilityReasons(measurements []measurement, values []MetricValue, requireCatalog bool) []string {
	for _, value := range values {
		if !value.Available {
			return []string{"metric is missing from one or more reports"}
		}
	}
	checks := []struct {
		label string
		value func(measurement) string
	}{
		{label: "unit", value: func(item measurement) string { return item.unit }},
		{label: "protocol", value: func(item measurement) string { return item.protocol }},
		{label: "provider", value: func(item measurement) string { return item.provider }},
		{label: "node/target", value: func(item measurement) string { return item.target }},
	}
	reasons := make([]string, 0, 5)
	for _, check := range checks {
		base := normalizeDimension(check.value(measurements[0]))
		if base == "" {
			reasons = append(reasons, check.label+" is unknown")
			continue
		}
		for i := 1; i < len(measurements); i++ {
			if normalizeDimension(check.value(measurements[i])) != base {
				reasons = append(reasons, check.label+" differs")
				break
			}
		}
	}
	if requireCatalog {
		base := normalizeDimension(measurements[0].catalogRevision)
		if base == "" {
			reasons = append(reasons, "catalog revision is unknown")
		} else {
			for i := 1; i < len(measurements); i++ {
				if normalizeDimension(measurements[i].catalogRevision) != base {
					reasons = append(reasons, "catalog revision differs")
					break
				}
			}
		}
	}
	return reasons
}

func addNumber(out map[string]measurement, key string, item measurement, raw any) {
	value, ok := numberValue(raw)
	if !ok || !isFinite(value) {
		return
	}
	item.value = value
	addMeasurement(out, key, item)
}

func addMeasurement(out map[string]measurement, key string, item measurement) {
	if !isFinite(item.value) {
		return
	}
	out[key] = item
}

func measurementKey(section, identity, metric string) string {
	return strings.ToLower(strings.TrimSpace(section)) + "\x00" + strings.ToLower(strings.TrimSpace(identity)) + "\x00" + strings.ToLower(strings.TrimSpace(metric))
}

func catalogRevision(root map[string]any) string {
	for _, key := range []string{"catalog_revision", "node_catalog_revision"} {
		if value := stringValue(root[key]); value != "" {
			return value
		}
	}
	if config, ok := asMap(root["config"]); ok {
		for _, key := range []string{"catalog_revision", "node_catalog_revision"} {
			if value := stringValue(config[key]); value != "" {
				return value
			}
		}
	}
	for _, key := range []string{"catalog", "node_catalog"} {
		if catalog, ok := asMap(root[key]); ok {
			if value := firstString(catalog, "revision", "id"); value != "" {
				return value
			}
		}
	}
	return ""
}

func reportTime(root map[string]any) time.Time {
	for _, key := range []string{"started_at", "timestamp", "finished_at"} {
		if value := stringValue(root[key]); value != "" {
			if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
				return parsed.UTC()
			}
		}
	}
	for _, key := range []string{"started_time", "finished_time"} {
		if value, ok := numberValue(root[key]); ok && value > 0 {
			return time.Unix(int64(value), 0).UTC()
		}
	}
	return time.Time{}
}

func reportLabel(root map[string]any, index int) string {
	if system, ok := asMap(root["system"]); ok {
		if cpu, ok := asMap(system["cpu"]); ok {
			if model := stringValue(cpu["model"]); model != "" {
				return truncate(model, 28)
			}
		}
	}
	if id := stringValue(root["report_id"]); id != "" {
		return truncate(id, 18)
	}
	return fmt.Sprintf("Report %d", index+1)
}

func updateContext(ctx metricContext, object map[string]any) metricContext {
	if provider := firstString(object, "provider", "source"); provider != "" {
		ctx.provider = provider
	}
	if protocol := firstString(object, "protocol", "method"); protocol != "" {
		ctx.protocol = protocol
	}
	if target := firstString(object, "node", "endpoint"); target != "" {
		ctx.target = target
	} else if target := stringValue(object["target"]); target != "" {
		ctx.target = target
	} else if target, ok := asMap(object["target"]); ok {
		if value := firstString(target, "endpoint", "id", "name"); value != "" {
			ctx.target = value
		}
	}
	return ctx
}

func genericUnit(field string, object map[string]any) string {
	fieldLower := strings.ToLower(field)
	if unit := stringValue(object["unit"]); unit != "" {
		return unit
	}
	if fieldLower == "throughput_per_sec" {
		if unit := stringValue(object["throughput_unit"]); unit != "" {
			return unit
		}
	}
	switch {
	case strings.HasSuffix(fieldLower, "_mbps"):
		return "Mbps"
	case strings.HasSuffix(fieldLower, "_ms"):
		return "ms"
	case strings.Contains(fieldLower, "ns_per") || strings.HasSuffix(fieldLower, "_ns"):
		return "ns/op"
	case strings.HasSuffix(fieldLower, "_bytes"):
		return "bytes"
	case strings.Contains(fieldLower, "percent") || strings.Contains(fieldLower, "loss"):
		return "%"
	default:
		return "count"
	}
}

func directionFor(field, unit string) direction {
	value := strings.ToLower(strings.TrimSpace(field + " " + unit))
	if strings.Contains(value, "latency") || strings.Contains(value, "jitter") || strings.Contains(value, "time") || strings.Contains(value, "loss") || strings.Contains(value, "ms") || strings.Contains(value, "ns/op") {
		return directionLower
	}
	if strings.Contains(value, "throughput") || strings.Contains(value, "mbps") || strings.Contains(value, "bandwidth") || strings.Contains(value, "available") || strings.Contains(value, "unlocked") {
		return directionHigher
	}
	return directionNeutral
}

func protocolForSection(section string) string {
	switch strings.ToLower(section) {
	case "hardware":
		return "benchmark"
	case "route":
		return "traceroute"
	case "ping":
		return "tcp-connect"
	case "speed":
		return "speed-probe"
	case "mail":
		return "tcp-connect"
	case "media":
		return "https"
	case "ip_quality":
		return "https+dnsbl"
	default:
		return "vmbench-section"
	}
}

func speedProtocol(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "cloudflare":
		return "https"
	case "iperf3":
		return "iperf3"
	case "speedtest_net", "speedtest_cn":
		return "speedtest-cli"
	default:
		return "speed-probe"
	}
}

func hardwareProvider(name string) string {
	lower := strings.ToLower(name)
	for _, provider := range []string{"sysbench", "openssl", "fio", "winsat", "geekbench", "stream", "mbw", "dd"} {
		if strings.Contains(lower, provider) {
			return provider
		}
	}
	return "vmbench"
}

func sectionRequiresCatalog(section string) bool {
	lower := strings.ToLower(section)
	return strings.Contains(lower, "route") || strings.Contains(lower, "ping") || strings.Contains(lower, "speed") || strings.Contains(lower, "node") || strings.Contains(lower, "network")
}

func skipGenericField(field string) bool {
	switch strings.ToLower(field) {
	case "enabled", "status", "message", "error", "detail", "id", "name", "title", "region", "city", "carrier",
		"provider", "provider_label", "source", "protocol", "probe_protocol", "probe_tool", "method", "target", "node", "endpoint", "kind", "unit",
		"throughput_unit", "started_time", "updated_time", "finished_time", "finish_time", "started_at", "finished_at",
		"schema_version", "version", "report_id", "duration_ms", "samples_ms", "port", "http_status", "as", "asn", "ttl", "iterations":
		return true
	default:
		return false
	}
}

func arrayIdentity(value any, index int) string {
	if object, ok := asMap(value); ok {
		if identity := firstString(object, "id", "name", "provider", "title"); identity != "" {
			return identity
		}
		if target, ok := asMap(object["target"]); ok {
			if identity := firstString(target, "id", "name", "endpoint"); identity != "" {
				return identity
			}
		}
		if port, ok := numberValue(object["port"]); ok {
			return "port-" + strconv.Itoa(int(port))
		}
	}
	return strconv.Itoa(index + 1)
}

func looksLikeSuite(root map[string]any) bool {
	if strings.EqualFold(stringValue(root["report_kind"]), "suite") {
		return true
	}
	if _, hasConfig := root["config"]; !hasConfig {
		return false
	}
	for _, key := range []string{"hardware", "route", "ping", "speed", "ip_quality", "mail", "media", "sections"} {
		if _, exists := root[key]; exists {
			return true
		}
	}
	return false
}

func looksLikeSection(section map[string]any) bool {
	for _, key := range []string{"enabled", "status", "result", "results"} {
		if _, ok := section[key]; ok {
			return true
		}
	}
	return false
}

func isEnvelopeKey(key string) bool {
	switch key {
	case "schema_version", "report_kind", "report_id", "app", "system", "config", "version", "timestamp",
		"started_at", "finished_at", "duration_ms", "started_time", "updated_time", "finished_time", "status", "message", "warnings",
		"catalog", "node_catalog", "catalog_revision", "node_catalog_revision":
		return true
	default:
		return false
	}
}

func joinHeaders(headers []Header) string {
	parts := make([]string, len(headers))
	for i, header := range headers {
		parts[i] = fmt.Sprintf("Report %d (%s)", i+1, header.Label)
	}
	return strings.Join(parts, "\t")
}

func printHeaderRow(w *tabwriter.Writer, label string, headers []Header, value func(Header) string) {
	row := label + "\t"
	for _, header := range headers {
		row += value(header) + "\t"
	}
	fmt.Fprintln(w, row)
}

func formatDelta(base, target float64, direction direction) string {
	if !isFinite(base) || !isFinite(target) || base == 0 {
		return "N/A"
	}
	percent := (target - base) / math.Abs(base) * 100
	switch direction {
	case directionLower:
		percent = -percent
	case directionNeutral:
		return fmt.Sprintf("%+.1f%%", percent)
	}
	if math.Abs(percent) < 0.05 {
		return "="
	}
	if percent > 0 {
		return fmt.Sprintf("▲%+.1f%%", percent)
	}
	return fmt.Sprintf("▼%+.1f%%", percent)
}

func formatMeasured(value float64, unit string) string {
	if !isFinite(value) {
		return "-"
	}
	if math.Abs(value) >= 100 {
		return fmt.Sprintf("%.0f %s", value, unit)
	}
	return fmt.Sprintf("%.2f %s", value, unit)
}

func asMap(value any) (map[string]any, bool) {
	result, ok := value.(map[string]any)
	return result, ok
}

func stringValue(value any) string {
	result, _ := value.(string)
	return strings.TrimSpace(result)
}

func boolValue(value any) (bool, bool) {
	result, ok := value.(bool)
	return result, ok
}

func numberValue(value any) (float64, bool) {
	switch number := value.(type) {
	case json.Number:
		result, err := number.Float64()
		return result, err == nil
	case float64:
		return number, true
	case float32:
		return float64(number), true
	case int:
		return float64(number), true
	case int64:
		return float64(number), true
	default:
		return 0, false
	}
}

func firstString(object map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := stringValue(object[key]); value != "" {
			return value
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func endpointWithPort(endpoint string, rawPort any) string {
	port, ok := numberValue(rawPort)
	if endpoint == "" || !ok || port <= 0 || port > 65535 || math.Trunc(port) != port {
		return endpoint
	}
	return net.JoinHostPort(endpoint, strconv.Itoa(int(port)))
}

func speedTargetIdentity(item map[string]any, fallback string) string {
	nodeID := firstString(item, "node_id")
	endpoint := firstString(item, "endpoint")
	if nodeID != "" && endpoint != "" {
		return nodeID + "@" + endpoint
	}
	return firstNonEmpty(nodeID, endpoint, firstString(item, "node", "target", "region"), fallback)
}

func normalizeDimension(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func truncate(value string, limit int) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit]) + "…"
}

func isFinite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
