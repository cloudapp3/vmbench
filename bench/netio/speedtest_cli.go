package netio

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// SpeedtestCLIResult stores one CLI-based speedtest outcome.
type SpeedtestCLIResult struct {
	Provider     string        `json:"provider"`
	NodeID       string        `json:"node_id,omitempty"`
	Node         string        `json:"node,omitempty"`
	Endpoint     string        `json:"endpoint,omitempty"`
	Region       string        `json:"region,omitempty"`
	DownloadMbps float64       `json:"download_mbps,omitempty"`
	UploadMbps   float64       `json:"upload_mbps,omitempty"`
	LatencyMs    float64       `json:"latency_ms,omitempty"`
	Elapsed      time.Duration `json:"elapsed,omitempty"`
}

// ProbeSpeedtestNet runs an Ookla-compatible speedtest CLI and parses JSON.
func ProbeSpeedtestNet(ctx context.Context) (*SpeedtestCLIResult, error) {
	return probeSpeedtestCommand(ctx, "speedtest_net", []string{"speedtest", "--format=json", "--accept-license", "--accept-gdpr"})
}

// ProbeSpeedtestCN runs a speedtest.cn-compatible CLI and parses JSON.
func ProbeSpeedtestCN(ctx context.Context) (*SpeedtestCLIResult, error) {
	candidates := [][]string{
		{"speedtest-cn", "--json"},
		{"speedtest.cn", "--json"},
		{"stc", "--json"},
	}
	var lastErr error
	for _, args := range candidates {
		result, err := probeSpeedtestCommand(ctx, "speedtest_cn", args)
		if err == nil {
			return result, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

func probeSpeedtestCommand(ctx context.Context, provider string, args []string) (*SpeedtestCLIResult, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("%s: empty command", provider)
	}
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	start := time.Now()
	out, err := exec.CommandContext(ctx, args[0], args[1:]...).CombinedOutput()
	elapsed := time.Since(start)
	if err != nil {
		return nil, fmt.Errorf("%s: %w: %s", provider, err, strings.TrimSpace(string(out)))
	}
	result, err := parseSpeedtestJSON(provider, out)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", provider, err)
	}
	result.Elapsed = elapsed
	return result, nil
}

func parseSpeedtestJSON(provider string, data []byte) (*SpeedtestCLIResult, error) {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	result := &SpeedtestCLIResult{Provider: provider}

	if summary := mapValue(raw, "summary"); summary != nil {
		if result.DownloadMbps == 0 {
			result.DownloadMbps = normalizeMbps(firstFloat(summary, "download", "download_mbps", "downloadMbps", "dl", "dl_mbps"))
		}
		if result.UploadMbps == 0 {
			result.UploadMbps = normalizeMbps(firstFloat(summary, "upload", "upload_mbps", "uploadMbps", "ul", "ul_mbps"))
		}
		if result.LatencyMs == 0 {
			result.LatencyMs = firstFloat(summary, "ping", "latency", "latency_ms", "latencyMs")
		}
		if result.Node == "" {
			result.Node = firstString(summary, "server", "node", "host", "name")
		}
		if result.Region == "" {
			result.Region = firstString(summary, "region", "location", "city", "country")
		}
	}

	if download := mapValue(raw, "download"); download != nil {
		result.DownloadMbps = bitsToMbps(firstFloat(download, "bandwidth", "bytes_per_second", "bytesPerSecond") * 8)
		if result.DownloadMbps == 0 {
			result.DownloadMbps = bitsToMbps(firstFloat(download, "bits_per_second", "bitsPerSecond", "bps"))
		}
	}
	if result.DownloadMbps == 0 {
		result.DownloadMbps = normalizeMbps(firstFloat(raw, "download", "download_mbps", "downloadMbps", "dl", "dl_mbps"))
	}

	if upload := mapValue(raw, "upload"); upload != nil {
		result.UploadMbps = bitsToMbps(firstFloat(upload, "bandwidth", "bytes_per_second", "bytesPerSecond") * 8)
		if result.UploadMbps == 0 {
			result.UploadMbps = bitsToMbps(firstFloat(upload, "bits_per_second", "bitsPerSecond", "bps"))
		}
	}
	if result.UploadMbps == 0 {
		result.UploadMbps = normalizeMbps(firstFloat(raw, "upload", "upload_mbps", "uploadMbps", "ul", "ul_mbps"))
	}

	if ping := mapValue(raw, "ping"); ping != nil {
		result.LatencyMs = firstFloat(ping, "latency", "jitter", "ms")
	}
	if result.LatencyMs == 0 {
		result.LatencyMs = firstFloat(raw, "ping", "latency", "latency_ms", "latencyMs")
	}

	if server := mapValue(raw, "server"); server != nil {
		result.NodeID = scalarString(server["id"])
		if host := firstString(server, "host"); host != "" {
			result.Endpoint = host
			if port := firstPositiveInt(server, "port"); port > 0 && !hasExplicitPort(host) {
				result.Endpoint = net.JoinHostPort(host, strconv.Itoa(port))
			}
		}
		result.Node = firstString(server, "name", "sponsor", "host")
		location := firstString(server, "location", "city", "country")
		if location != "" {
			result.Region = location
		}
	}
	if result.Node == "" {
		result.Node = firstString(raw, "server", "node", "host")
	}
	if result.Region == "" {
		result.Region = firstString(raw, "region", "location", "city", "country")
	}

	if strings.TrimSpace(result.Node) == "" {
		result.Node = firstString(raw, "server_name", "serverName")
	}

	if result.DownloadMbps == 0 && result.UploadMbps == 0 && result.LatencyMs == 0 {
		return nil, fmt.Errorf("missing speed metrics in JSON")
	}
	return result, nil
}

func mapValue(raw map[string]any, key string) map[string]any {
	value, ok := raw[key]
	if !ok {
		return nil
	}
	asMap, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	return asMap
}

func firstFloat(raw map[string]any, keys ...string) float64 {
	for _, key := range keys {
		value, ok := raw[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case float64:
			return typed
		case int:
			return float64(typed)
		case json.Number:
			f, _ := typed.Float64()
			return f
		case string:
			f, _ := strconv.ParseFloat(strings.TrimSpace(typed), 64)
			return f
		}
	}
	return 0
}

func firstString(raw map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := raw[key]
		if !ok {
			continue
		}
		if text, ok := value.(string); ok {
			return strings.TrimSpace(text)
		}
	}
	return ""
}

func scalarString(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case float64:
		if typed >= 0 && typed == math.Trunc(typed) {
			return strconv.FormatInt(int64(typed), 10)
		}
	case json.Number:
		return strings.TrimSpace(typed.String())
	}
	return ""
}

func firstPositiveInt(raw map[string]any, keys ...string) int {
	for _, key := range keys {
		value, ok := raw[key]
		if !ok {
			continue
		}
		text := scalarString(value)
		parsed, err := strconv.Atoi(text)
		if err == nil && parsed > 0 && parsed <= 65535 {
			return parsed
		}
	}
	return 0
}

func hasExplicitPort(host string) bool {
	_, _, err := net.SplitHostPort(strings.TrimSpace(host))
	return err == nil
}

func bitsToMbps(bits float64) float64 {
	if bits <= 0 {
		return 0
	}
	return bits / 1_000_000
}

func normalizeMbps(value float64) float64 {
	if value <= 0 {
		return 0
	}
	if value > 100_000 {
		return bitsToMbps(value)
	}
	return value
}
