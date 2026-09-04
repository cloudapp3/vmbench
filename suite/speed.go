package suite

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/cloudapp3/vmbench/bench/netio"
)

func runSpeedSection(ctx context.Context, opts Options, report *SuiteReport) {
	section := &report.Speed
	section.Status = "running"
	section.StartedTime = time.Now().Unix()

	result := &SpeedResult{
		Providers: make([]SpeedProviderResult, 0, len(opts.SpeedProviders)+len(opts.IperfHosts)),
		Groups:    make([]SpeedProviderGroup, 0, len(opts.SpeedProviders)),
	}
	aggregate := &SpeedSummary{}
	available := 0
	failed := 0

	for _, provider := range opts.SpeedProviders {
		group := buildSpeedGroup(ctx, provider, opts)
		if group.ID == "" {
			continue
		}
		result.Groups = append(result.Groups, group)
		result.Providers = append(result.Providers, group.Providers...)
		available += group.Available
		failed += group.Failed
		if group.Summary != nil {
			mergeSpeedSummary(aggregate, group.Summary.Provider, group.Summary.ProviderLabel, group.Summary.Node, group.Summary.Region, group.Summary.DownloadMbps, group.Summary.UploadMbps, group.Summary.LatencyMs)
		}
	}

	aggregate.Available = available
	aggregate.Failed = failed
	aggregate.SelectedProviders = len(opts.SpeedProviders)
	if len(opts.SpeedProviders) > 1 {
		aggregate.Provider = "aggregate"
		aggregate.ProviderLabel = "Best per metric"
		aggregate.Aggregation = "best_per_metric"
		aggregate.Node = ""
		aggregate.Region = ""
	} else {
		aggregate.Aggregation = "provider"
	}
	result.Summary = aggregate

	section.Result = result
	section.FinishTime = time.Now().Unix()
	if available == 0 {
		if failed == 0 {
			section.Status = "skipped"
			section.Message = "no speed providers configured"
			return
		}
		section.Status = "error"
		section.Message = "no speed providers succeeded"
		return
	}

	section.Status = statusFromCounts(available, failed)
	msgParts := make([]string, 0, 3)
	if result.Summary != nil && result.Summary.DownloadMbps > 0 {
		msgParts = append(msgParts, fmt.Sprintf("DL %.1f Mbps", result.Summary.DownloadMbps))
	}
	if result.Summary != nil && result.Summary.UploadMbps > 0 {
		msgParts = append(msgParts, fmt.Sprintf("UL %.1f Mbps", result.Summary.UploadMbps))
	}
	msgParts = append(msgParts, fmt.Sprintf("groups %d ok/%d failed", available, failed))
	section.Message = strings.Join(msgParts, " · ")
}

func buildSpeedGroup(ctx context.Context, provider string, opts Options) SpeedProviderGroup {
	switch provider {
	case SpeedProviderCloudflare:
		return buildCloudflareSpeedGroup(ctx)
	case SpeedProviderSpeedtestNet:
		return buildSpeedtestNetGroup(ctx)
	case SpeedProviderSpeedtestCN:
		return buildSpeedtestCNGroup(ctx)
	case SpeedProviderIperf3:
		return buildIperfSpeedGroup(ctx, opts.IperfHosts)
	case SpeedProviderChinaISP:
		return buildChinaISPGroup(ctx, opts)
	case SpeedProviderSpeedtestISP:
		return buildSpeedtestISPGroup(ctx)
	default:
		return SpeedProviderGroup{}
	}
}

var ispCarrierLabels = map[string]string{
	"telecom": "China Telecom",
	"unicom":  "China Unicom",
	"mobile":  "China Mobile",
}

// ispDownloadNodesForOptions resolves isp_download catalog nodes, preferring
// the caller-resolved catalog and falling back to the embedded snapshot.
func ispDownloadNodesForOptions(opts Options) []netio.SpeedNode {
	if opts.ResolvedCatalog != nil {
		return netio.ISPDownloadNodesFromManifest(*opts.ResolvedCatalog)
	}
	return netio.DefaultISPDownloadNodes()
}

// buildChinaISPGroup downloads one speedtest.cn endpoint per China carrier
// sequentially so carriers do not contend for bandwidth.
func buildChinaISPGroup(ctx context.Context, opts Options) SpeedProviderGroup {
	builder := newSpeedProviderGroupBuilder(SpeedProviderChinaISP, "China ISP (speedtest.cn)")
	nodes := ispDownloadNodesForOptions(opts)
	if len(nodes) == 0 {
		builder.addError("china-isp", "download", "", fmt.Errorf("no isp_download nodes in the selected node catalog"))
		return builder.finish()
	}
	for _, carrier := range netio.CarrierOrder() {
		var carrierNodes []netio.SpeedNode
		for _, node := range nodes {
			if node.Carrier == carrier {
				carrierNodes = append(carrierNodes, node)
			}
		}
		if len(carrierNodes) == 0 {
			builder.addError("china-isp-"+carrier, "download", "", fmt.Errorf("no %s node in the selected node catalog", carrier))
			continue
		}
		// Try same-carrier nodes in catalog order until one succeeds so a
		// single unreachable city does not fail the whole carrier.
		var (
			probe   *netio.DownloadProbeResult
			node    netio.SpeedNode
			lastErr error
			tried   []string
		)
		for _, candidate := range carrierNodes {
			if ctx.Err() != nil {
				break
			}
			probe, lastErr = netio.ProbeDownload(ctx, candidate)
			if lastErr == nil {
				node = candidate
				break
			}
			tried = append(tried, candidate.ID+": "+lastErr.Error())
		}
		if probe == nil || lastErr != nil {
			builder.addError("china-isp-"+carrier, "download", "", fmt.Errorf("all %s nodes failed: %s", carrier, strings.Join(tried, "; ")))
			continue
		}
		dlMbps := mebibytesPerSecondToMegabitsPerSecond(probe.ThroughputMiBPerSec())
		builder.addOK(SpeedProviderResult{
			ID:            "china-isp-" + carrier,
			Provider:      SpeedProviderChinaISP,
			ProviderLabel: "China ISP (speedtest.cn)",
			Kind:          "download",
			Status:        "ok",
			NodeID:        node.ID,
			Node:          node.Name,
			Endpoint:      speedNodeEndpoint(node),
			Region:        "China · " + node.City,
			DownloadMbps:  dlMbps,
			ElapsedMs:     durationMillis(probe.Elapsed),
		}, dlMbps, 0, 0)
	}
	return builder.finish()
}

// buildSpeedtestISPGroup pins the Ookla speedtest CLI to per-carrier China
// server IDs, trying fallback IDs when the preferred server fails.
func buildSpeedtestISPGroup(ctx context.Context) SpeedProviderGroup {
	builder := newSpeedProviderGroupBuilder(SpeedProviderSpeedtestISP, "China ISP (speedtest.net)")
	servers := netio.OoklaCarrierServers()
	for _, carrier := range netio.CarrierOrder() {
		ids := servers[carrier]
		if len(ids) == 0 {
			builder.addError("speedtest-isp-"+carrier, "download_upload", "", fmt.Errorf("no speedtest.net server IDs recorded for %s", carrier))
			continue
		}
		var (
			probe *netio.SpeedtestCLIResult
			err   error
		)
		for _, id := range ids {
			probe, err = netio.ProbeSpeedtestISPServer(ctx, id)
			if err == nil {
				break
			}
		}
		if err != nil {
			builder.addError("speedtest-isp-"+carrier, "download_upload", "", err)
			continue
		}
		item := speedtestProviderResult("speedtest-isp-"+carrier, SpeedProviderSpeedtestISP, "China ISP (speedtest.net)", probe)
		if item.Region == "" {
			item.Region = ispCarrierLabels[carrier]
		}
		builder.addOK(item, item.DownloadMbps, item.UploadMbps, item.LatencyMs)
	}
	return builder.finish()
}

func buildCloudflareSpeedGroup(ctx context.Context) SpeedProviderGroup {
	builder := newSpeedProviderGroupBuilder(SpeedProviderCloudflare, "Cloudflare")

	if multi, err := netio.ProbeMultiDownload(ctx); err != nil {
		builder.addError("cf-download", "download", "speed.cloudflare.com", err)
	} else {
		dlMbps := mebibytesPerSecondToMegabitsPerSecond(multi.ThroughputMiBPerSec())
		builder.addOK(SpeedProviderResult{
			ID:            "cf-download",
			Provider:      SpeedProviderCloudflare,
			ProviderLabel: "Cloudflare",
			Kind:          "download",
			Status:        "ok",
			Node:          "speed.cloudflare.com",
			DownloadMbps:  dlMbps,
			ElapsedMs:     durationMillis(multi.Elapsed),
		}, dlMbps, 0, 0)
	}

	if upload, err := netio.ProbeUpload(ctx); err != nil {
		builder.addError("cf-upload", "upload", "speed.cloudflare.com", err)
	} else {
		ulMbps := mebibytesPerSecondToMegabitsPerSecond(upload.ThroughputMiBPerSec())
		builder.addOK(SpeedProviderResult{
			ID:            "cf-upload",
			Provider:      SpeedProviderCloudflare,
			ProviderLabel: "Cloudflare",
			Kind:          "upload",
			Status:        "ok",
			Node:          "speed.cloudflare.com",
			UploadMbps:    ulMbps,
			ElapsedMs:     durationMillis(upload.Elapsed),
		}, 0, ulMbps, 0)
	}

	return builder.finish()
}

func buildSpeedtestNetGroup(ctx context.Context) SpeedProviderGroup {
	builder := newSpeedProviderGroupBuilder(SpeedProviderSpeedtestNet, "Speedtest.net")
	speedtest, err := netio.ProbeSpeedtestNet(ctx)
	if err != nil {
		builder.addError("speedtest-net", "download_upload", "", err)
		return builder.finish()
	}
	item := speedtestProviderResult("speedtest-net", SpeedProviderSpeedtestNet, "Speedtest.net", speedtest)
	builder.addOK(item, item.DownloadMbps, item.UploadMbps, item.LatencyMs)
	return builder.finish()
}

func buildSpeedtestCNGroup(ctx context.Context) SpeedProviderGroup {
	builder := newSpeedProviderGroupBuilder(SpeedProviderSpeedtestCN, "Speedtest.cn")
	speedtest, err := netio.ProbeSpeedtestCN(ctx)
	if err != nil {
		builder.addError("speedtest-cn", "download_upload", "", err)
		return builder.finish()
	}
	item := speedtestProviderResult("speedtest-cn", SpeedProviderSpeedtestCN, "Speedtest.cn", speedtest)
	builder.addOK(item, item.DownloadMbps, item.UploadMbps, item.LatencyMs)
	return builder.finish()
}

func buildIperfSpeedGroup(ctx context.Context, hosts []string) SpeedProviderGroup {
	builder := newSpeedProviderGroupBuilder(SpeedProviderIperf3, "iperf3")
	if len(hosts) == 0 {
		builder.addError("iperf3", "download", "", fmt.Errorf("no --iperf-host provided"))
		return builder.finish()
	}
	usableHosts := 0
	for _, host := range hosts {
		host = strings.TrimSpace(host)
		if host == "" {
			continue
		}
		usableHosts++
		probe, err := netio.ProbeIperf(ctx, host, 10)
		if err != nil {
			builder.addError("iperf-"+host, "download", host, err)
			continue
		}
		dlMbps := probe.ThroughputMbitPerSec()
		builder.addOK(SpeedProviderResult{
			ID:            "iperf-" + host,
			Provider:      SpeedProviderIperf3,
			ProviderLabel: "iperf3",
			Kind:          "download",
			Status:        "ok",
			Node:          host,
			DownloadMbps:  dlMbps,
			ElapsedMs:     durationMillis(probe.Elapsed),
		}, dlMbps, 0, 0)
	}
	if usableHosts == 0 {
		builder.addError("iperf3", "download", "", fmt.Errorf("no --iperf-host provided"))
	}
	return builder.finish()
}

type speedProviderGroupBuilder struct {
	group     SpeedProviderGroup
	available int
	failed    int
	summary   SpeedSummary
}

func newSpeedProviderGroupBuilder(provider, label string) *speedProviderGroupBuilder {
	return &speedProviderGroupBuilder{
		group: SpeedProviderGroup{
			ID:            provider,
			Provider:      provider,
			ProviderLabel: label,
			Providers:     make([]SpeedProviderResult, 0, 4),
		},
		summary: SpeedSummary{
			Provider:      provider,
			ProviderLabel: label,
			Aggregation:   "provider",
		},
	}
}

func (b *speedProviderGroupBuilder) addOK(item SpeedProviderResult, dlMbps, ulMbps, latencyMs float64) {
	b.group.Providers = append(b.group.Providers, item)
	b.available++
	mergeSpeedSummary(&b.summary, b.group.Provider, b.group.ProviderLabel, item.Node, item.Region, dlMbps, ulMbps, latencyMs)
}

func (b *speedProviderGroupBuilder) addError(id, kind, node string, err error) {
	b.group.Providers = append(b.group.Providers, SpeedProviderResult{
		ID:            id,
		Provider:      b.group.Provider,
		ProviderLabel: b.group.ProviderLabel,
		Kind:          kind,
		Status:        "error",
		Node:          node,
		Message:       err.Error(),
	})
	b.failed++
}

func (b *speedProviderGroupBuilder) finish() SpeedProviderGroup {
	b.group.Available = b.available
	b.group.Failed = b.failed
	b.group.Status = speedGroupStatus(b.available, b.failed)
	summary := b.summary
	summary.Available = b.available
	summary.Failed = b.failed
	b.group.Summary = &summary
	b.group.Message = speedGroupMessage(b.group.Summary, b.available, b.failed)
	return b.group
}

func speedGroupStatus(available, failed int) string {
	return statusFromCounts(available, failed)
}

func speedGroupMessage(summary *SpeedSummary, available, failed int) string {
	parts := make([]string, 0, 4)
	if summary != nil {
		if summary.DownloadMbps > 0 {
			parts = append(parts, fmt.Sprintf("DL %.1f Mbps", summary.DownloadMbps))
		}
		if summary.UploadMbps > 0 {
			parts = append(parts, fmt.Sprintf("UL %.1f Mbps", summary.UploadMbps))
		}
		if summary.LatencyMs > 0 {
			parts = append(parts, fmt.Sprintf("latency %.1f ms", summary.LatencyMs))
		}
	}
	switch {
	case available == 0 && failed == 0:
		parts = append(parts, "skipped")
	case failed == 0:
		parts = append(parts, fmt.Sprintf("%d ok", available))
	default:
		parts = append(parts, fmt.Sprintf("%d ok/%d failed", available, failed))
	}
	return strings.Join(parts, " · ")
}

func speedtestProviderResult(id, provider, label string, speedtest *netio.SpeedtestCLIResult) SpeedProviderResult {
	if speedtest == nil {
		return SpeedProviderResult{ID: id, Provider: provider, ProviderLabel: label, Status: "error", Message: "empty speedtest result"}
	}
	return SpeedProviderResult{
		ID:            id,
		Provider:      provider,
		ProviderLabel: label,
		Kind:          "download_upload",
		Status:        "ok",
		NodeID:        speedtest.NodeID,
		Node:          speedtest.Node,
		Endpoint:      speedtest.Endpoint,
		Region:        speedtest.Region,
		DownloadMbps:  speedtest.DownloadMbps,
		UploadMbps:    speedtest.UploadMbps,
		LatencyMs:     speedtest.LatencyMs,
		ElapsedMs:     durationMillis(speedtest.Elapsed),
	}
}

func mergeSpeedSummary(result *SpeedSummary, provider, label, node, region string, dlMbps, ulMbps, latencyMs float64) {
	if result == nil {
		return
	}
	if result.Provider == "" && (dlMbps > 0 || ulMbps > 0 || latencyMs > 0) {
		result.Provider = provider
		result.ProviderLabel = label
		result.Node = node
		result.Region = region
	}
	if dlMbps > result.DownloadMbps {
		result.Provider = provider
		result.ProviderLabel = label
		result.Node = node
		result.Region = region
		result.DownloadMbps = dlMbps
	}
	if ulMbps > result.UploadMbps {
		result.Provider = provider
		result.ProviderLabel = label
		result.Node = node
		result.Region = region
		result.UploadMbps = ulMbps
	}
	if result.LatencyMs == 0 || (latencyMs > 0 && latencyMs < result.LatencyMs) {
		result.LatencyMs = latencyMs
	}
}

func durationMillis(d time.Duration) float64 {
	if d <= 0 {
		return 0
	}
	return float64(d) / float64(time.Millisecond)
}

// speedNodeEndpoint extracts a host:port display endpoint from a node URL.
func speedNodeEndpoint(node netio.SpeedNode) string {
	if parsed, err := url.Parse(node.TestURL); err == nil && parsed.Host != "" {
		return parsed.Host
	}
	return node.TestURL
}

func mebibytesPerSecondToMegabitsPerSecond(value float64) float64 {
	return value * 8 * 1.048576
}
