package suite

import (
	"fmt"
	"html/template"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

var htmlTemplate = template.Must(template.New("suite-report").Funcs(template.FuncMap{
	"defaultText": func(value, fallback string) string {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
		return fallback
	},
	"formatUnix": func(value int64) string {
		if value <= 0 {
			return "-"
		}
		return time.Unix(value, 0).UTC().Format(time.RFC3339)
	},
	"formatTime": func(value time.Time) string {
		if value.IsZero() {
			return "-"
		}
		return value.UTC().Format(time.RFC3339)
	},
	"formatDurationMS": func(value int64) string {
		if value < 0 {
			return "-"
		}
		return (time.Duration(value) * time.Millisecond).String()
	},
	"formatFloat": func(value float64, suffix string) string {
		if value <= 0 {
			return "-"
		}
		if value >= 100 {
			return fmt.Sprintf("%.0f %s", value, suffix)
		}
		return fmt.Sprintf("%.2f %s", value, suffix)
	},
	"formatThroughput": func(value float64, unit string) string {
		if value <= 0 {
			return "-"
		}
		unit = strings.TrimSpace(unit)
		if unit == "" {
			return fmt.Sprintf("%.2f", value)
		}
		return fmt.Sprintf("%.2f %s", value, unit)
	},
	"formatBytes": func(value uint64) string {
		const (
			kib = uint64(1024)
			mib = kib * 1024
			gib = mib * 1024
			tib = gib * 1024
		)
		switch {
		case value >= tib:
			return fmt.Sprintf("%.2f TiB", float64(value)/float64(tib))
		case value >= gib:
			return fmt.Sprintf("%.2f GiB", float64(value)/float64(gib))
		case value >= mib:
			return fmt.Sprintf("%.2f MiB", float64(value)/float64(mib))
		case value >= kib:
			return fmt.Sprintf("%.2f KiB", float64(value)/float64(kib))
		default:
			return fmt.Sprintf("%d B", value)
		}
	},
	"statusClass": func(status string) string {
		switch strings.ToLower(strings.TrimSpace(status)) {
		case "ok", "reachable", "open", "available", "direct":
			return "ok"
		case "partial", "refused", "mixed", "timeout":
			return "warn"
		case "error", "failed", "no_response", "unreachable", "blocked", "invalid", "http_error":
			return "err"
		default:
			return "muted"
		}
	},
	"boolText": func(value bool) string {
		if value {
			return "yes"
		}
		return "no"
	},
	"traceStatus": func(value RouteRun) string {
		return value.EffectiveStatus()
	},
	"traceReached": traceDestinationReachedText,
	"sectionNames": func(value SectionSelector) string {
		return strings.Join(value.Names(), ", ")
	},
	"formatEndpoint": func(host string, port int) string {
		host = strings.TrimSpace(host)
		if host == "" || port <= 0 {
			return host
		}
		return net.JoinHostPort(host, strconv.Itoa(port))
	},
}).Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>VMBench Suite Report</title>
<style>
:root { color-scheme:light; --bg:#f6f7f9; --surface:#fff; --line:#dfe3e8; --text:#17202a; --muted:#65717e; --ok:#08783f; --okbg:#eaf7ef; --warn:#7a5200; --warnbg:#fff6d8; --err:#a72b2b; --errbg:#fff0f0; --accent:#1565c0; }
* { box-sizing:border-box; }
body { margin:0; background:var(--bg); color:var(--text); font:14px/1.55 system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif; }
main { width:min(1500px,calc(100% - 32px)); margin:24px auto 48px; }
h1,h2,h3 { margin:0; letter-spacing:0; }
h1 { font-size:28px; }
h2 { margin-bottom:12px; font-size:19px; }
h3 { margin:18px 0 8px; font-size:15px; }
p { margin:6px 0; }
.hero,.section { margin-bottom:16px; padding:18px; background:var(--surface); border:1px solid var(--line); border-radius:8px; }
.hero-head { display:flex; align-items:flex-start; justify-content:space-between; gap:16px; }
.grid { display:grid; grid-template-columns:repeat(auto-fit,minmax(180px,1fr)); gap:12px; margin-top:14px; }
.metric { min-width:0; padding-top:8px; border-top:2px solid var(--line); }
.metric strong { display:block; overflow-wrap:anywhere; font-size:17px; }
.label,.small { color:var(--muted); font-size:12px; }
.badge { display:inline-block; padding:2px 8px; border-radius:999px; font-size:12px; font-weight:700; }
.badge.ok { color:var(--ok); background:var(--okbg); }
.badge.warn { color:var(--warn); background:var(--warnbg); }
.badge.err { color:var(--err); background:var(--errbg); }
.badge.muted { color:var(--muted); background:#edf0f3; }
.table-wrap { width:100%; overflow-x:auto; }
table { width:100%; border-collapse:collapse; }
th,td { padding:9px 8px; border-bottom:1px solid var(--line); text-align:left; vertical-align:top; overflow-wrap:anywhere; }
th { color:var(--muted); background:#fafbfc; font-size:11px; text-transform:uppercase; }
tr:last-child td { border-bottom:0; }
code { font:12px/1.5 ui-monospace,SFMono-Regular,Consolas,monospace; color:#303b46; }
.error { color:var(--err); }
.subsection { margin-top:18px; padding-top:14px; border-top:1px solid var(--line); }
ul { margin:8px 0 0; padding-left:20px; }
@media (max-width:640px) { main { width:100%; margin:0; } .hero,.section { margin:0; border-width:0 0 1px; border-radius:0; padding:14px; } .hero-head { display:block; } .hero-head .badge { margin-top:8px; } th,td { padding:8px 6px; } }
</style>
</head>
<body><main>
<section class="hero">
  <div class="hero-head">
    <div><h1>VMBench Suite</h1><p class="small">Raw measurements and structured diagnostics</p></div>
    <span class="badge {{ statusClass .Status }}">{{ defaultText .Status "unknown" }}</span>
  </div>
  <p>{{ defaultText .Message "-" }}</p>
  <div class="grid">
    <div class="metric"><span class="label">Report ID</span><strong><code>{{ defaultText .ReportID "legacy" }}</code></strong></div>
    <div class="metric"><span class="label">Schema</span><strong>{{ .SchemaVersion }}</strong></div>
    <div class="metric"><span class="label">App</span><strong>{{ defaultText .App.Version "unknown" }}</strong></div>
    <div class="metric"><span class="label">Started</span><strong>{{ if .StartedAt.IsZero }}{{ formatUnix .StartedTime }}{{ else }}{{ formatTime .StartedAt }}{{ end }}</strong></div>
    <div class="metric"><span class="label">Finished</span><strong>{{ if .FinishedAt.IsZero }}{{ formatUnix .FinishedTime }}{{ else }}{{ formatTime .FinishedAt }}{{ end }}</strong></div>
    <div class="metric"><span class="label">Duration</span><strong>{{ formatDurationMS .DurationMS }}</strong></div>
  </div>
</section>

<section class="section">
  <h2>System and Configuration</h2>
  <div class="grid">
    <div class="metric"><span class="label">Host</span><strong>{{ defaultText .System.OS.Hostname "-" }}</strong><span class="small">{{ defaultText .System.OS.Name "unknown OS" }} / {{ defaultText .System.OS.Kernel "unknown kernel" }}</span></div>
    <div class="metric"><span class="label">CPU</span><strong>{{ defaultText .System.CPU.Model "-" }}</strong><span class="small">{{ .System.CPU.PhysicalCores }} cores / {{ .System.CPU.LogicalCores }} threads / {{ defaultText .System.CPU.Arch "-" }}</span></div>
    <div class="metric"><span class="label">Memory</span><strong>{{ formatBytes .System.Memory.TotalBytes }}</strong><span class="small">{{ defaultText .System.Memory.Type "type unknown" }} / {{ .System.Memory.FreqMHz }} MHz / {{ .System.Memory.Channels }} channels</span></div>
    <div class="metric"><span class="label">Virtualization</span><strong>{{ defaultText .System.Virtualization.System "unknown" }}</strong><span class="small">role {{ defaultText .System.Virtualization.Role "unknown" }}</span></div>
    <div class="metric"><span class="label">Preset</span><strong>{{ defaultText .Config.Preset "custom" }}</strong><span class="small">IP {{ defaultText .Config.IPVersion "v4" }} / {{ .Config.Iterations }} iterations</span></div>
    <div class="metric"><span class="label">Sections</span><strong>{{ sectionNames .Config.Sections }}</strong><span class="small">timeout {{ .Config.TimeoutMS }} ms</span></div>
    <div class="metric"><span class="label">Node Catalog</span><strong>{{ defaultText .Config.CatalogRevision "not used" }}</strong><span class="small">source {{ defaultText .Config.CatalogSource "-" }} / {{ len .Config.NodeIDs }} selected nodes</span></div>
  </div>
  {{ if .Config.NodeIDs }}<p class="small"><strong>Selected node IDs:</strong> {{ range .Config.NodeIDs }}<code>{{ . }}</code> {{ end }}</p>{{ end }}
  <p class="small">Build commit {{ defaultText .App.Commit "unknown" }} / build time {{ defaultText .App.BuildTime "unknown" }}</p>
</section>

<section class="section">
  <h2>Section Status</h2>
  <div class="table-wrap"><table><thead><tr><th>Section</th><th>Enabled</th><th>Status</th><th>Message</th></tr></thead><tbody>
    <tr><td>Hardware</td><td>{{ boolText .Hardware.Enabled }}</td><td><span class="badge {{ statusClass .Hardware.Status }}">{{ defaultText .Hardware.Status "unknown" }}</span></td><td>{{ defaultText .Hardware.Message "-" }}</td></tr>
    <tr><td>Network Info</td><td>{{ boolText .NetworkInfo.Enabled }}</td><td><span class="badge {{ statusClass .NetworkInfo.Status }}">{{ defaultText .NetworkInfo.Status "unknown" }}</span></td><td>{{ defaultText .NetworkInfo.Message "-" }}</td></tr>
    <tr><td>Route</td><td>{{ boolText .Route.Enabled }}</td><td><span class="badge {{ statusClass .Route.Status }}">{{ defaultText .Route.Status "unknown" }}</span></td><td>{{ defaultText .Route.Message "-" }}</td></tr>
    <tr><td>Ping</td><td>{{ boolText .Ping.Enabled }}</td><td><span class="badge {{ statusClass .Ping.Status }}">{{ defaultText .Ping.Status "unknown" }}</span></td><td>{{ defaultText .Ping.Message "-" }}</td></tr>
    <tr><td>Speed</td><td>{{ boolText .Speed.Enabled }}</td><td><span class="badge {{ statusClass .Speed.Status }}">{{ defaultText .Speed.Status "unknown" }}</span></td><td>{{ defaultText .Speed.Message "-" }}</td></tr>
    <tr><td>IP Quality</td><td>{{ boolText .IPQuality.Enabled }}</td><td><span class="badge {{ statusClass .IPQuality.Status }}">{{ defaultText .IPQuality.Status "unknown" }}</span></td><td>{{ defaultText .IPQuality.Message "-" }}</td></tr>
    <tr><td>Reachability</td><td>{{ boolText .Reachability.Enabled }}</td><td><span class="badge {{ statusClass .Reachability.Status }}">{{ defaultText .Reachability.Status "unknown" }}</span></td><td>{{ defaultText .Reachability.Message "-" }}</td></tr>
    <tr><td>Mail</td><td>{{ boolText .Mail.Enabled }}</td><td><span class="badge {{ statusClass .Mail.Status }}">{{ defaultText .Mail.Status "unknown" }}</span></td><td>{{ defaultText .Mail.Message "-" }}</td></tr>
    <tr><td>Media</td><td>{{ boolText .Media.Enabled }}</td><td><span class="badge {{ statusClass .Media.Status }}">{{ defaultText .Media.Status "unknown" }}</span></td><td>{{ defaultText .Media.Message "-" }}</td></tr>
  </tbody></table></div>
</section>

{{ with .Hardware.Report }}
<section class="section">
  <h2>Hardware Workloads</h2>
  <div class="table-wrap"><table><thead><tr><th>Name</th><th>Category</th><th>Iterations</th><th>Median</th><th>Throughput</th><th>Latency</th><th>Detail</th><th>Error</th></tr></thead><tbody>
  {{ range .Results.Workloads }}<tr><td>{{ .Name }}</td><td>{{ .Category }}</td>{{ with .Result }}<td>{{ .Iterations }}</td><td>{{ formatFloat .MedianMS "ms" }}</td><td>{{ formatThroughput .ThroughputPerSec .ThroughputUnit }}</td><td>{{ formatFloat .AvgNSPerAccess "ns" }}</td><td>{{ defaultText .Detail "-" }}</td><td class="error">{{ defaultText .Error "-" }}</td>{{ else }}<td>-</td><td>-</td><td>-</td><td>-</td><td>-</td><td class="error">missing result</td>{{ end }}</tr>{{ end }}
  {{ range .Extensions.Workloads }}<tr><td>{{ .Name }}</td><td>{{ .Category }}</td>{{ with .Result }}<td>{{ .Iterations }}</td><td>{{ formatFloat .MedianMS "ms" }}</td><td>{{ formatThroughput .ThroughputPerSec .ThroughputUnit }}</td><td>{{ formatFloat .AvgNSPerAccess "ns" }}</td><td>{{ defaultText .Detail "-" }}</td><td class="error">{{ defaultText .Error "-" }}</td>{{ else }}<td>-</td><td>-</td><td>-</td><td>-</td><td>-</td><td class="error">missing result</td>{{ end }}</tr>{{ end }}
  </tbody></table></div>
</section>
{{ end }}

{{ with .NetworkInfo.Result }}
<section class="section">
  <h2>Network Identity</h2>
  <h3>Public Addresses</h3>
  <div class="table-wrap"><table><thead><tr><th>Family</th><th>Address</th><th>ASN</th><th>Organization</th><th>ISP</th><th>Country</th></tr></thead><tbody>
    {{ with .PublicIPv4 }}<tr><td>{{ .IPVersion }}</td><td><code>{{ .IP }}</code></td><td>{{ .ASN }}</td><td>{{ defaultText .Org "-" }}</td><td>{{ defaultText .ISP "-" }}</td><td>{{ defaultText .CountryCode .Country }}</td></tr>{{ end }}
    {{ with .PublicIPv6 }}<tr><td>{{ .IPVersion }}</td><td><code>{{ .IP }}</code></td><td>{{ .ASN }}</td><td>{{ defaultText .Org "-" }}</td><td>{{ defaultText .ISP "-" }}</td><td>{{ defaultText .CountryCode .Country }}</td></tr>{{ end }}
  </tbody></table></div>
  {{ if .LocalGlobalAddresses }}<h3>Local Addresses</h3><div class="table-wrap"><table><thead><tr><th>Interface</th><th>Family</th><th>Address</th><th>Private</th></tr></thead><tbody>{{ range .LocalGlobalAddresses }}<tr><td>{{ .Interface }}</td><td>{{ .IPVersion }}</td><td><code>{{ .Address }}</code></td><td>{{ boolText .Private }}</td></tr>{{ end }}</tbody></table></div>{{ end }}
  {{ if .NAT }}<h3>NAT Heuristic</h3><div class="table-wrap"><table><thead><tr><th>Family</th><th>Status</th><th>Method</th><th>Public IP</th><th>Local IP</th><th>Reason</th></tr></thead><tbody>{{ range .NAT }}<tr><td>{{ .IPVersion }}</td><td><span class="badge {{ statusClass .Status }}">{{ .Status }}</span></td><td>{{ .Method }}</td><td><code>{{ defaultText .PublicIP "-" }}</code></td><td><code>{{ defaultText .LocalIP "-" }}</code></td><td>{{ .Reason }}</td></tr>{{ end }}</tbody></table></div>{{ end }}
  {{ if .Providers }}<h3>Evidence Providers</h3><div class="table-wrap"><table><thead><tr><th>ID</th><th>Kind</th><th>Family</th><th>Status</th><th>Error</th></tr></thead><tbody>{{ range .Providers }}<tr><td>{{ .ID }}</td><td>{{ .Kind }}</td><td>{{ defaultText .IPVersion "-" }}</td><td><span class="badge {{ statusClass .Status }}">{{ .Status }}</span></td><td class="error">{{ defaultText .Error "-" }}</td></tr>{{ end }}</tbody></table></div>{{ end }}
</section>
{{ end }}

{{ if .Route.Results }}
<section class="section"><h2>Route Evidence</h2>
{{ range .Route.Results }}
	  <div class="subsection"><h3>{{ .Target.Name }} <span class="badge {{ statusClass (traceStatus .) }}">{{ traceStatus . }}</span></h3><p class="small"><code>{{ defaultText .Target.ID "legacy" }}</code> / {{ .Target.City }} / {{ .Target.Carrier }} / AS{{ .Target.AS }} / {{ defaultText .Target.IPFamily "-" }} / catalog {{ defaultText .Target.Protocol "-" }} / probe {{ defaultText .ProbeProtocol "unknown" }} via {{ defaultText .ProbeTool "unknown" }} / {{ defaultText .Target.Source "-" }} / requested <code>{{ formatEndpoint .Target.Endpoint .Target.Port }}</code> / resolved <code>{{ defaultText .ResolvedTarget "unknown" }}</code> / destination reached {{ traceReached .DestinationReached }}</p>
  {{ if .Error }}<p class="error">{{ .Error }}</p>{{ end }}
  {{ if .Hops }}<div class="table-wrap"><table><thead><tr><th>TTL</th><th>IP</th><th>RTT</th><th>Timeout</th></tr></thead><tbody>{{ range .Hops }}<tr><td>{{ .TTL }}</td><td><code>{{ defaultText .IP "-" }}</code></td><td>{{ formatFloat .RTTMs "ms" }}</td><td>{{ boolText .Timeout }}</td></tr>{{ end }}</tbody></table></div>{{ end }}</div>
{{ end }}
</section>
{{ end }}

{{ if .Ping.Results }}
<section class="section"><h2>Ping Evidence</h2><div class="table-wrap"><table><thead><tr><th>ID</th><th>Target</th><th>City</th><th>Carrier</th><th>Family</th><th>Connection</th><th>Average</th><th>Jitter</th><th>Loss</th><th>Sent/Received</th><th>Status</th><th>Message</th></tr></thead><tbody>
{{ range .Ping.Results }}<tr><td>{{ defaultText .ID "-" }}<br><span class="small">catalog {{ defaultText .Protocol "-" }} / probe {{ defaultText .ProbeProtocol "unknown" }} via {{ defaultText .ProbeTool "unknown" }} / {{ defaultText .Source "-" }}</span></td><td>{{ defaultText .Name .Target }}<br><code>{{ formatEndpoint .Target .Port }}</code></td><td>{{ defaultText .City "-" }}</td><td>{{ defaultText .Carrier "-" }} / AS{{ .ASN }}</td><td>{{ defaultText .IPFamily "-" }}</td><td><span class="badge {{ statusClass .ConnectionState }}">{{ defaultText .ConnectionState "unknown" }}</span></td><td>{{ formatFloat .AvgLatencyMs "ms" }}</td><td>{{ formatFloat .JitterMs "ms" }}</td><td>{{ printf "%.1f%%" .PacketLoss }}</td><td>{{ .Sent }}/{{ .Received }}</td><td><span class="badge {{ statusClass .Status }}">{{ defaultText .Status "unknown" }}</span></td><td>{{ defaultText .Message "-" }}</td></tr>{{ end }}
</tbody></table></div></section>
{{ end }}

{{ with .Speed.Result }}
<section class="section"><h2>Speed Evidence</h2>
  {{ with .Summary }}<div class="grid"><div class="metric"><span class="label">Download</span><strong>{{ formatFloat .DownloadMbps "Mbps" }}</strong></div><div class="metric"><span class="label">Upload</span><strong>{{ formatFloat .UploadMbps "Mbps" }}</strong></div><div class="metric"><span class="label">Latency</span><strong>{{ formatFloat .LatencyMs "ms" }}</strong></div><div class="metric"><span class="label">Aggregation</span><strong>{{ defaultText .Aggregation "single provider" }}</strong></div></div>{{ end }}
  {{ if .Groups }}<h3>Provider Groups</h3><div class="table-wrap"><table><thead><tr><th>Provider</th><th>Status</th><th>Available</th><th>Failed</th><th>Download</th><th>Upload</th><th>Latency</th><th>Message</th></tr></thead><tbody>{{ range .Groups }}<tr><td>{{ defaultText .ProviderLabel .Provider }}</td><td><span class="badge {{ statusClass .Status }}">{{ defaultText .Status "unknown" }}</span></td><td>{{ .Available }}</td><td>{{ .Failed }}</td>{{ with .Summary }}<td>{{ formatFloat .DownloadMbps "Mbps" }}</td><td>{{ formatFloat .UploadMbps "Mbps" }}</td><td>{{ formatFloat .LatencyMs "ms" }}</td>{{ else }}<td>-</td><td>-</td><td>-</td>{{ end }}<td>{{ defaultText .Message "-" }}</td></tr>{{ end }}</tbody></table></div>{{ end }}
  {{ if .Providers }}<h3>Provider Results</h3><div class="table-wrap"><table><thead><tr><th>ID</th><th>Provider</th><th>Kind</th><th>Node</th><th>Endpoint</th><th>Region</th><th>Download</th><th>Upload</th><th>Latency</th><th>Elapsed</th><th>Status</th><th>Message</th></tr></thead><tbody>{{ range .Providers }}<tr><td>{{ .ID }}</td><td>{{ defaultText .ProviderLabel .Provider }}</td><td>{{ defaultText .Kind "-" }}</td><td>{{ defaultText .Node .NodeID }}{{ if .NodeID }}<br><code>{{ .NodeID }}</code>{{ end }}</td><td>{{ defaultText .Endpoint "-" }}</td><td>{{ defaultText .Region "-" }}</td><td>{{ formatFloat .DownloadMbps "Mbps" }}</td><td>{{ formatFloat .UploadMbps "Mbps" }}</td><td>{{ formatFloat .LatencyMs "ms" }}</td><td>{{ formatFloat .ElapsedMs "ms" }}</td><td><span class="badge {{ statusClass .Status }}">{{ defaultText .Status "unknown" }}</span></td><td>{{ defaultText .Message "-" }}</td></tr>{{ end }}</tbody></table></div>{{ end }}
</section>
{{ end }}

{{ if .Reachability.Results }}
<section class="section"><h2>Website and Telegram Reachability</h2><div class="table-wrap"><table><thead><tr><th>ID</th><th>Category</th><th>Protocol</th><th>Endpoint</th><th>Latency</th><th>HTTP</th><th>Status</th><th>Error</th></tr></thead><tbody>{{ range .Reachability.Results }}<tr><td>{{ .ID }}</td><td>{{ .Category }}</td><td>{{ .Protocol }}</td><td><code>{{ .Endpoint }}</code></td><td>{{ formatFloat .LatencyMs "ms" }}</td><td>{{ if .HTTPStatus }}{{ .HTTPStatus }}{{ else }}-{{ end }}</td><td><span class="badge {{ statusClass .Status }}">{{ .Status }}</span></td><td class="error">{{ defaultText .Error "-" }}</td></tr>{{ end }}</tbody></table></div></section>
{{ end }}

{{ with .IPQuality.Result }}
<section class="section"><h2>IP Quality</h2>
  {{ with .BasicInfo }}<div class="grid"><div class="metric"><span class="label">IP</span><strong><code>{{ defaultText .IP "-" }}</code></strong><span class="small">source {{ defaultText .Source "unknown" }}</span></div><div class="metric"><span class="label">ASN</span><strong>{{ .ASN }}</strong><span class="small">{{ defaultText .Org .ISP }}</span></div><div class="metric"><span class="label">Location</span><strong>{{ defaultText .CountryCode .Country }}</strong></div><div class="metric"><span class="label">Flags</span><strong>hosting={{ boolText .Hosting }} / proxy={{ boolText .Proxy }}</strong><span class="error">{{ .Error }}</span></div></div>{{ end }}
  {{ with .RiskSummary }}<h3>Risk Evidence</h3><p>{{ defaultText .Summary "-" }}</p><div class="table-wrap"><table><tbody><tr><th>Risk level</th><td>{{ defaultText .RiskLevel "unknown" }}</td><th>DNSBL</th><td>{{ boolText .DNSBLSupported }} via {{ defaultText .DNSBLTool "-" }}</td><th>Listed</th><td>{{ .DNSBLListedCount }} {{ range .DNSBLListed }}<code>{{ . }}</code> {{ end }}</td></tr><tr><th>DNSBL detail</th><td colspan="5">{{ defaultText .DNSBLMessage "-" }}</td></tr></tbody></table></div>{{ end }}
  {{ with .Score }}<p><strong>Business risk diagnostic: {{ .Total }}/{{ .MaxTotal }} ({{ .Level }})</strong></p>{{ end }}
  {{ if .MailPorts }}<h3>IP Quality Mail Probe</h3><div class="table-wrap"><table><thead><tr><th>Port</th><th>Target</th><th>Method</th><th>Latency</th><th>Status</th><th>Message</th></tr></thead><tbody>{{ range .MailPorts }}<tr><td>{{ .Port }}</td><td>{{ defaultText .Target "-" }}</td><td>{{ defaultText .Method "-" }}</td><td>{{ formatFloat .LatencyMs "ms" }}</td><td><span class="badge {{ statusClass .Status }}">{{ defaultText .Status "unknown" }}</span></td><td>{{ defaultText .Message "-" }}</td></tr>{{ end }}</tbody></table></div>{{ end }}
</section>
{{ end }}

{{ if .Mail.Results }}
<section class="section"><h2>Mail Port Reachability</h2><div class="table-wrap"><table><thead><tr><th>Port</th><th>Title</th><th>Target</th><th>Method</th><th>Supported</th><th>Latency</th><th>Status</th><th>Message</th></tr></thead><tbody>{{ range .Mail.Results }}<tr><td>{{ .Port }}</td><td>{{ defaultText .Title "-" }}</td><td>{{ defaultText .Target "-" }}</td><td>{{ defaultText .Method "-" }}</td><td>{{ boolText .Supported }}</td><td>{{ formatFloat .LatencyMs "ms" }}</td><td><span class="badge {{ statusClass .Status }}">{{ defaultText .Status "unknown" }}</span></td><td>{{ defaultText .Message "-" }}</td></tr>{{ end }}</tbody></table></div></section>
{{ end }}

{{ with .Media.Result }}{{ if .Items }}
<section class="section"><h2>Media Availability</h2><p class="small">Available {{ .Summary.Available }} / blocked {{ .Summary.Blocked }} / unknown {{ .Summary.Unknown }}</p><div class="table-wrap"><table><thead><tr><th>ID</th><th>Service</th><th>Region</th><th>Status</th><th>Message</th></tr></thead><tbody>{{ range .Items }}<tr><td>{{ .ID }}</td><td>{{ .Title }}</td><td>{{ defaultText .Region "-" }}</td><td><span class="badge {{ statusClass .Status }}">{{ .Status }}</span></td><td>{{ defaultText .Message "-" }}</td></tr>{{ end }}</tbody></table></div></section>
{{ end }}{{ end }}

{{ if .Warnings }}<section class="section"><h2>Warnings</h2><ul>{{ range .Warnings }}<li>{{ . }}</li>{{ end }}</ul></section>{{ end }}
</main></body></html>`))

// WriteHTML renders a self-contained suite report.
func WriteHTML(w io.Writer, report SuiteReport) error {
	return htmlTemplate.Execute(w, report)
}
