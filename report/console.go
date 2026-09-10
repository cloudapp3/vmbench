package report

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/cloudapp3/vmbench/i18n"
	"github.com/cloudapp3/vmbench/sysinfo"
	"github.com/cloudapp3/vmbench/textgrid"
)

// WriteConsole writes a human-readable summary to w.
func WriteConsole(w io.Writer, doc Document) error {
	if w == nil {
		w = os.Stdout
	}
	line := strings.Repeat("═", 62)
	if _, err := fmt.Fprintf(w, "%s\n  %s\n%s\n", line, i18n.Tf("report.console.title", map[string]any{"Version": doc.Version}), line); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(w, "  %s: %s (%dC/%dT)\n", i18n.PadCells(i18n.T("report.label.cpu"), 9), doc.System.CPU.Model, doc.System.CPU.PhysicalCores, doc.System.CPU.LogicalCores)
	_, _ = fmt.Fprintf(w, "  %s: %.1f GB %s\n", i18n.PadCells(i18n.T("report.label.memory"), 9), float64(doc.System.Memory.TotalBytes)/(1024*1024*1024), doc.System.Memory.Type)
	_, _ = fmt.Fprintf(w, "  %s: %s (%s)\n", i18n.PadCells(i18n.T("report.label.os"), 9), doc.System.OS.Name, doc.System.OS.Kernel)
	_, _ = fmt.Fprintf(w, "  %s: %s\n", i18n.PadCells(i18n.T("report.label.go"), 9), doc.System.OS.GoVersion)
	if cache := sysinfo.FormatCacheLine(doc.System.CPU.CacheSizes); cache != "" {
		_, _ = fmt.Fprintf(w, "  %s: %s\n", i18n.PadCells(i18n.T("report.label.cache"), 9), cache)
	}
	if nic := doc.System.Network.PrimaryNIC(); nic != "" {
		_, _ = fmt.Fprintf(w, "  %s: %s\n", i18n.PadCells(i18n.T("report.label.nic"), 9), nic)
	}
	if oversell := doc.System.Platform.OversellSignalsText(); oversell != "" {
		_, _ = fmt.Fprintf(w, "  %s: %s\n", i18n.PadCells(i18n.T("report.label.oversell"), 9), oversell)
	}
	_, _ = fmt.Fprintf(w, "%s\n\n", line)

	writeWorkloadTable(w, i18n.T("report.console.measured"), doc.Results.Workloads, line)
	if len(doc.Extensions.Workloads) > 0 {
		writeWorkloadTable(w, i18n.T("report.console.extensions"), doc.Extensions.Workloads, line)
	}

	if len(doc.Warnings) > 0 {
		_, _ = fmt.Fprintf(w, "\n%s:\n", i18n.T("report.console.warnings"))
		for _, warning := range doc.Warnings {
			_, _ = fmt.Fprintf(w, "  - %s\n", warning)
		}
	}
	return nil
}

func writeWorkloadTable(w io.Writer, title string, entries []WorkloadEntry, line string) {
	if len(entries) == 0 {
		return
	}
	_, _ = fmt.Fprintf(w, "%s\n  %s\n%s\n", line, title, line)
	headers := []string{
		i18n.T("report.col.workload"),
		i18n.T("report.col.category"),
		i18n.T("report.col.time"),
		i18n.T("report.col.throughput"),
		i18n.T("report.col.latency"),
		i18n.T("report.col.result"),
	}
	rows := make([][]string, 0, len(entries))
	for _, item := range entries {
		rows = append(rows, []string{
			item.Name,
			item.Category,
			formatTime(item.Result),
			formatThroughput(item.Result),
			formatLatency(item.Result),
			formatDetail(item.Result),
		})
	}
	_, _ = fmt.Fprint(w, textgrid.Render(headers, rows, 2))
}

func formatTime(result *ResultEntry) string {
	if result == nil {
		return "-"
	}
	if result.Error != "" && result.MedianMS == 0 {
		return "ERR"
	}
	return fmt.Sprintf("%.1fms", result.MedianMS)
}

func formatThroughput(result *ResultEntry) string {
	if result == nil || result.ThroughputPerSec <= 0 {
		return "-"
	}
	unit := strings.TrimSpace(result.ThroughputUnit)
	if unit == "" {
		unit = "ops/s"
	}
	if result.ThroughputPerSec >= 100 {
		return fmt.Sprintf("%.0f %s", result.ThroughputPerSec, unit)
	}
	return fmt.Sprintf("%.2f %s", result.ThroughputPerSec, unit)
}

func formatLatency(result *ResultEntry) string {
	if result == nil || result.AvgNSPerAccess <= 0 {
		return "-"
	}
	metric := fmt.Sprintf("%.2f ns/op", result.AvgNSPerAccess)
	if result.LatencyP99NS > 0 {
		metric += " (p99 " + formatLatencyP99NS(result.LatencyP99NS) + ")"
	}
	return metric
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

func formatDetail(result *ResultEntry) string {
	if result == nil {
		return "-"
	}
	if strings.TrimSpace(result.Error) != "" {
		return "ERR: " + strings.TrimSpace(result.Error)
	}
	if strings.TrimSpace(result.Detail) != "" {
		return strings.TrimSpace(result.Detail)
	}
	return i18n.T("report.console.ok")
}
