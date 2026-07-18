package report

import (
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
)

// WriteConsole writes a human-readable summary to w.
func WriteConsole(w io.Writer, doc Document) error {
	if w == nil {
		w = os.Stdout
	}
	line := strings.Repeat("═", 62)
	if _, err := fmt.Fprintf(w, "%s\n  VMBench %s   —   System Benchmark\n%s\n", line, doc.Version, line); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(w, "  CPU      : %s (%dC/%dT)\n", doc.System.CPU.Model, doc.System.CPU.PhysicalCores, doc.System.CPU.LogicalCores)
	_, _ = fmt.Fprintf(w, "  Memory   : %.1f GB %s\n", float64(doc.System.Memory.TotalBytes)/(1024*1024*1024), doc.System.Memory.Type)
	_, _ = fmt.Fprintf(w, "  OS       : %s (%s)\n", doc.System.OS.Name, doc.System.OS.Kernel)
	_, _ = fmt.Fprintf(w, "  Go       : %s\n", doc.System.OS.GoVersion)
	_, _ = fmt.Fprintf(w, "%s\n\n", line)

	writeWorkloadTable(w, "Measured Workloads", doc.Results.Workloads, line)
	if len(doc.Extensions.Workloads) > 0 {
		writeWorkloadTable(w, "Extensions", doc.Extensions.Workloads, line)
	}

	if len(doc.Warnings) > 0 {
		_, _ = fmt.Fprintln(w, "\nWarnings:")
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
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "Workload\tCategory\tTime\tThroughput\tLatency\tResult")
	for _, item := range entries {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
			item.Name,
			item.Category,
			formatTime(item.Result),
			formatThroughput(item.Result),
			formatLatency(item.Result),
			formatDetail(item.Result),
		)
	}
	_ = tw.Flush()
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
	return fmt.Sprintf("%.2f ns/op", result.AvgNSPerAccess)
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
	return "ok"
}
