package report

import (
	"fmt"
	"html/template"
	"io"
	"sort"
	"strings"
)

var htmlTemplate = template.Must(template.New("report").Funcs(template.FuncMap{
	"divGB":       func(value uint64) float64 { return float64(value) / (1024 * 1024 * 1024) },
	"categories":  categoriesFromDoc,
	"workloadsIn": workloadsInCategory,
	"formatTime":  formatHTMLTime,
	"formatRate":  formatHTMLThroughput,
	"formatLat":   formatHTMLLatency,
	"formatText":  formatHTMLDetail,
}).Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>VMBench Report</title>
<style>
:root {
  --primary: #3b82f6;
  --success: #10b981;
  --danger:  #ef4444;
  --gray-50: #f9fafb; --gray-100: #f3f4f6; --gray-200: #e5e7eb;
  --gray-500: #6b7280; --gray-700: #374151; --gray-900: #111827;
}
* { box-sizing: border-box; }
body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", system-ui, sans-serif; max-width: 980px; margin: 0 auto; padding: 24px 16px; color: var(--gray-900); background: #fff; line-height: 1.5; }
.hero { padding: 32px 0 18px; text-align: center; }
.hero h1 { margin: 0 0 8px; font-size: 32px; }
.hero-sub { color: var(--gray-500); font-size: 14px; }
.sys-cards { display: grid; grid-template-columns: repeat(auto-fit, minmax(180px, 1fr)); gap: 12px; margin: 20px 0; }
.sys-card { border: 1px solid var(--gray-200); border-radius: 12px; padding: 16px; text-align: center; background: var(--gray-50); }
.sys-card .icon { font-size: 22px; margin-bottom: 2px; }
.sys-card .label { font-size: 11px; color: var(--gray-500); text-transform: uppercase; letter-spacing: 0.5px; }
.sys-card .value { font-weight: 600; margin: 4px 0; font-size: 14px; word-break: break-word; }
.sys-card .detail { font-size: 13px; color: var(--gray-500); }
.section-title { font-size: 18px; font-weight: 600; margin: 28px 0 12px; padding-bottom: 8px; border-bottom: 2px solid var(--gray-200); }
table { border-collapse: collapse; width: 100%; margin: 8px 0 20px; font-size: 14px; }
th { background: var(--gray-50); color: var(--gray-700); font-size: 12px; text-transform: uppercase; letter-spacing: .04em; }
th, td { border-bottom: 1px solid var(--gray-200); text-align: left; padding: 10px 12px; vertical-align: top; }
td.metric { font-weight: 600; font-variant-numeric: tabular-nums; white-space: nowrap; }
td.error { color: var(--danger); }
.footer { margin-top: 32px; padding-top: 16px; border-top: 1px solid var(--gray-200); color: var(--gray-500); font-size: 12px; display: flex; justify-content: space-between; flex-wrap: wrap; gap: 8px; }
</style>
</head>
<body>

<div class="hero">
  <h1>VMBench Report</h1>
  <div class="hero-sub">{{ .System.CPU.Model }}</div>
</div>

<div class="sys-cards">
  <div class="sys-card">
    <div class="icon">&#9881;</div>
    <div class="label">CPU</div>
    <div class="value">{{ .System.CPU.Model }}</div>
    <div class="detail">{{ .System.CPU.PhysicalCores }}C / {{ .System.CPU.LogicalCores }}T</div>
  </div>
  <div class="sys-card">
    <div class="icon">&#9776;</div>
    <div class="label">Memory</div>
    <div class="value">{{ printf "%.1f" (divGB .System.Memory.TotalBytes) }} GB</div>
    <div class="detail">{{ .System.Memory.Type }}</div>
  </div>
  <div class="sys-card">
    <div class="icon">&#9783;</div>
    <div class="label">OS</div>
    <div class="value">{{ .System.OS.Name }}</div>
    <div class="detail">{{ .System.OS.Kernel }}</div>
  </div>
  <div class="sys-card">
    <div class="icon">&#9889;</div>
    <div class="label">Go</div>
    <div class="value">{{ .System.OS.GoVersion }}</div>
    <div class="detail">&nbsp;</div>
  </div>
</div>

{{ range $cat := categories . }}
<div class="section-title">{{ $cat }}</div>
<table>
<thead><tr><th>Workload</th><th>Time</th><th>Throughput</th><th>Latency</th><th>Result</th></tr></thead>
<tbody>
{{ range workloadsIn $.Results.Workloads $cat }}
<tr>
  <td>{{ .Name }}</td>
  <td class="metric">{{ formatTime .Result }}</td>
  <td class="metric">{{ formatRate .Result }}</td>
  <td class="metric">{{ formatLat .Result }}</td>
  <td class="{{ if and .Result .Result.Error }}error{{ end }}">{{ formatText .Result }}</td>
</tr>
{{ end }}
</tbody>
</table>
{{ end }}

{{ if .Extensions.Workloads }}
<div class="section-title">Extensions</div>
<table>
<thead><tr><th>Workload</th><th>Category</th><th>Time</th><th>Throughput</th><th>Result</th></tr></thead>
<tbody>
{{ range .Extensions.Workloads }}
<tr>
  <td>{{ .Name }}</td>
  <td>{{ .Category }}</td>
  <td class="metric">{{ formatTime .Result }}</td>
  <td class="metric">{{ formatRate .Result }}</td>
  <td class="{{ if and .Result .Result.Error }}error{{ end }}">{{ formatText .Result }}</td>
</tr>
{{ end }}
</tbody>
</table>
{{ end }}

{{ if .Warnings }}
<div class="section-title">Warnings</div>
<ul>
{{ range .Warnings }}
<li>{{ . }}</li>
{{ end }}
</ul>
{{ end }}

<div class="footer">
  <span>VMBench {{ .Version }}</span>
  <span>{{ .Timestamp }}</span>
</div>
</body>
</html>`))

// WriteHTML writes a standalone HTML report.
func WriteHTML(w io.Writer, doc Document) error {
	return htmlTemplate.Execute(w, doc)
}

func categoriesFromDoc(doc Document) []string {
	seen := map[string]struct{}{}
	for _, item := range doc.Results.Workloads {
		seen[item.Category] = struct{}{}
	}
	cats := make([]string, 0, len(seen))
	for cat := range seen {
		cats = append(cats, cat)
	}
	sort.Strings(cats)
	return cats
}

func workloadsInCategory(entries []WorkloadEntry, category string) []WorkloadEntry {
	var out []WorkloadEntry
	for _, e := range entries {
		if e.Category == category {
			out = append(out, e)
		}
	}
	return out
}

func formatHTMLTime(result *ResultEntry) string {
	if result == nil {
		return "-"
	}
	if result.Error != "" && result.MedianMS == 0 {
		return "ERR"
	}
	return fmt.Sprintf("%.1f ms", result.MedianMS)
}

func formatHTMLThroughput(result *ResultEntry) string {
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

func formatHTMLLatency(result *ResultEntry) string {
	if result == nil || result.AvgNSPerAccess <= 0 {
		return "-"
	}
	return fmt.Sprintf("%.2f ns/op", result.AvgNSPerAccess)
}

func formatHTMLDetail(result *ResultEntry) string {
	if result == nil {
		return "-"
	}
	if strings.TrimSpace(result.Error) != "" {
		return strings.TrimSpace(result.Error)
	}
	if strings.TrimSpace(result.Detail) != "" {
		return strings.TrimSpace(result.Detail)
	}
	return "ok"
}
