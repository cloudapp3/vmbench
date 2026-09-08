package report

import (
	"fmt"
	"io"
	"os"
	"slices"
	"sort"
	"strings"

	"github.com/cloudapp3/vmbench/i18n"
	"github.com/cloudapp3/vmbench/textgrid"
)

// WriteCompare writes a side-by-side comparison of two or more reports.
func WriteCompare(w io.Writer, docs []Document) error {
	if len(docs) < 2 {
		return fmt.Errorf("at least 2 reports required for comparison")
	}
	if w == nil {
		w = os.Stdout
	}

	line := strings.Repeat("═", 62)
	fmt.Fprintf(w, "%s\n  %s\n%s\n\n", line, i18n.T("report.compare.title"), line)

	headers := append([]string{i18n.T("report.compare.property")}, reportHeaders(docs)...)
	var sysRows [][]string
	sysRows = appendSysRow(sysRows, i18n.T("report.label.cpu"), docs, func(d Document) string {
		return fmt.Sprintf("%s (%dC/%dT)", d.System.CPU.Model, d.System.CPU.PhysicalCores, d.System.CPU.LogicalCores)
	})
	sysRows = appendSysRow(sysRows, i18n.T("report.label.memory"), docs, func(d Document) string {
		return fmt.Sprintf("%.1f GB %s", float64(d.System.Memory.TotalBytes)/(1024*1024*1024), d.System.Memory.Type)
	})
	sysRows = appendSysRow(sysRows, i18n.T("report.label.os"), docs, func(d Document) string {
		return fmt.Sprintf("%s (%s)", d.System.OS.Name, d.System.OS.Kernel)
	})
	fmt.Fprint(w, textgrid.Render(headers, sysRows, 2))
	if warnings := comparabilityWarnings(docs); len(warnings) > 0 {
		fmt.Fprintf(w, "\n%s:\n", i18n.T("report.compare.warnings"))
		for _, warning := range warnings {
			fmt.Fprintf(w, "  - %s\n", warning)
		}
	}

	fmt.Fprintf(w, "\n%s\n  %s\n%s\n", line, i18n.T("report.compare.workloadDetails"), line)
	headers = append([]string{i18n.T("report.col.workload"), i18n.T("report.col.metric")}, reportHeaders(docs)...)
	headers = append(headers, i18n.T("report.compare.delta"))

	var rows [][]string
	workloadMap := buildWorkloadMap(docs)
	for _, name := range sortedKeys(workloadMap) {
		entries := workloadMap[name]
		rows = appendMetricRow(rows, name, "time", docs, entries, func(r *ResultEntry) float64 {
			if r == nil {
				return 0
			}
			return r.MedianMS
		}, "ms", true)
		rows = appendThroughputRow(rows, docs, entries)
		rows = appendMetricRow(rows, "", "latency", docs, entries, func(r *ResultEntry) float64 {
			if r == nil {
				return 0
			}
			return r.AvgNSPerAccess
		}, "ns/op", true)
	}
	fmt.Fprint(w, textgrid.Render(headers, rows, 2))

	_, _ = fmt.Fprintln(w)
	return nil
}

func comparabilityWarnings(docs []Document) []string {
	if len(docs) < 2 {
		return nil
	}
	base := docs[0].Config
	warnings := make([]string, 0, 4)
	for i := 1; i < len(docs); i++ {
		current := docs[i].Config
		if current.Iterations != base.Iterations {
			warnings = append(warnings, fmt.Sprintf("report 1 uses %d iterations; report %d uses %d", base.Iterations, i+1, current.Iterations))
		}
		if displayMode(current.Mode) != displayMode(base.Mode) {
			warnings = append(warnings, fmt.Sprintf("report 1 mode is %q; report %d mode is %q", displayMode(base.Mode), i+1, displayMode(current.Mode)))
		}
		if current.Scope != base.Scope {
			warnings = append(warnings, fmt.Sprintf("report 1 scope is %q; report %d scope is %q", displayScope(base.Scope), i+1, displayScope(current.Scope)))
		}
		if !slices.Equal(current.HardwareTools, base.HardwareTools) {
			warnings = append(warnings, fmt.Sprintf("report 1 and report %d use different hardware tool selections", i+1))
		}
		if !slices.Equal(current.IperfHosts, base.IperfHosts) {
			warnings = append(warnings, fmt.Sprintf("report 1 and report %d use different iperf targets", i+1))
		}
	}
	for i, doc := range docs {
		seen := make(map[string]struct{})
		for _, workload := range append(doc.Results.Workloads, doc.Extensions.Workloads...) {
			if _, exists := seen[workload.Name]; exists {
				warnings = append(warnings, fmt.Sprintf("report %d contains duplicate workload %q", i+1, workload.Name))
			}
			seen[workload.Name] = struct{}{}
		}
	}
	return warnings
}

func displayScope(scope string) string {
	if scope = strings.TrimSpace(scope); scope != "" {
		return scope
	}
	return "unknown/legacy"
}

// displayMode maps the absent mode field of post-removal reports to the
// "single" value every normalized legacy report carried.
func displayMode(mode string) string {
	if mode = strings.TrimSpace(mode); mode != "" {
		return mode
	}
	return "single"
}

func appendMetricRow(
	rows [][]string,
	name string,
	metric string,
	docs []Document,
	entries map[int]WorkloadEntry,
	value func(*ResultEntry) float64,
	unit string,
	lowerIsBetter bool,
) [][]string {
	values := make([]float64, len(docs))
	any := false
	for i := range docs {
		if e, ok := entries[i]; ok && e.Result != nil && strings.TrimSpace(e.Result.Error) == "" {
			values[i] = value(e.Result)
			if values[i] > 0 {
				any = true
			}
		}
	}
	if !any {
		return rows
	}
	row := []string{name, metric}
	for _, v := range values {
		if v > 0 {
			row = append(row, formatMeasured(v, unit))
		} else {
			row = append(row, "-")
		}
	}
	row = append(row, formatDelta(values[0], values[len(values)-1], lowerIsBetter))
	return append(rows, row)
}

func appendThroughputRow(rows [][]string, docs []Document, entries map[int]WorkloadEntry) [][]string {
	values := make([]float64, len(docs))
	units := make([]string, len(docs))
	commonUnit := ""
	unitInitialized := false
	compatible := true
	any := false
	for i := range docs {
		entry, ok := entries[i]
		if !ok || entry.Result == nil || strings.TrimSpace(entry.Result.Error) != "" || entry.Result.ThroughputPerSec <= 0 {
			continue
		}
		values[i] = entry.Result.ThroughputPerSec
		units[i] = strings.TrimSpace(entry.Result.ThroughputUnit)
		any = true
		if !unitInitialized {
			commonUnit = units[i]
			unitInitialized = true
		} else if units[i] != commonUnit {
			compatible = false
		}
	}
	if !any {
		return rows
	}
	row := []string{"", "throughput"}
	for i, value := range values {
		row = append(row, formatMeasured(value, units[i]))
	}
	if compatible {
		row = append(row, formatDelta(values[0], values[len(values)-1], throughputLowerIsBetter(commonUnit)))
	} else {
		row = append(row, i18n.T("report.compare.incompatibleUnits"))
	}
	return append(rows, row)
}

func reportHeaders(docs []Document) []string {
	parts := make([]string, len(docs))
	for i := range docs {
		parts[i] = fmt.Sprintf("%s %d (%s)", i18n.T("report.compare.reportN"), i+1, shortCPU(docs[i]))
	}
	return parts
}

func shortCPU(d Document) string {
	model := d.System.CPU.Model
	if len(model) > 20 {
		model = model[:20] + "…"
	}
	return model
}

func appendSysRow(rows [][]string, label string, docs []Document, fn func(Document) string) [][]string {
	row := []string{label}
	for _, d := range docs {
		row = append(row, fn(d))
	}
	return append(rows, row)
}

func formatDelta(base, target float64, lowerIsBetter bool) string {
	if base <= 0 || target <= 0 {
		return "-"
	}
	pct := (target - base) / base * 100
	if lowerIsBetter {
		pct = -pct
	}
	if pct > 0.05 {
		return fmt.Sprintf("▲%+.1f%%", pct)
	}
	if pct < -0.05 {
		return fmt.Sprintf("▼%+.1f%%", pct)
	}
	return "="
}

func formatMeasured(value float64, unit string) string {
	if value <= 0 {
		return "-"
	}
	unit = strings.TrimSpace(unit)
	if unit == "" {
		return fmt.Sprintf("%.2f", value)
	}
	if value >= 100 {
		return fmt.Sprintf("%.0f %s", value, unit)
	}
	return fmt.Sprintf("%.2f %s", value, unit)
}

func throughputLowerIsBetter(unit string) bool {
	switch strings.ToLower(strings.TrimSpace(unit)) {
	case "ms", "ms avg", "latency ms", "loss %", "% loss":
		return true
	default:
		return false
	}
}

func buildWorkloadMap(docs []Document) map[string]map[int]WorkloadEntry {
	result := map[string]map[int]WorkloadEntry{}
	for i, d := range docs {
		for _, w := range append(d.Results.Workloads, d.Extensions.Workloads...) {
			if _, ok := result[w.Name]; !ok {
				result[w.Name] = map[int]WorkloadEntry{}
			}
			result[w.Name][i] = w
		}
	}
	return result
}

func sortedKeys(m map[string]map[int]WorkloadEntry) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
