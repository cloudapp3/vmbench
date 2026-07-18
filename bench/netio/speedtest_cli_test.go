package netio

import (
	"context"
	"testing"
	"time"
)

func TestParseSpeedtestJSON(t *testing.T) {
	data := []byte(`{
		"download": {"bandwidth": 12500000},
		"upload": {"bits_per_second": 80000000},
		"ping": {"latency": 10.5},
		"server": {"name": "Tokyo", "location": "JP"}
	}`)

	got, err := parseSpeedtestJSON("speedtest_net", data)
	if err != nil {
		t.Fatalf("parseSpeedtestJSON() error = %v", err)
	}
	if got.DownloadMbps != 100 {
		t.Fatalf("DownloadMbps = %v, want 100", got.DownloadMbps)
	}
	if got.UploadMbps != 80 {
		t.Fatalf("UploadMbps = %v, want 80", got.UploadMbps)
	}
	if got.LatencyMs != 10.5 {
		t.Fatalf("LatencyMs = %v, want 10.5", got.LatencyMs)
	}
	if got.Node != "Tokyo" || got.Region != "JP" {
		t.Fatalf("Node/Region = %q/%q, want Tokyo/JP", got.Node, got.Region)
	}
}

func TestParseSpeedtestJSONSummaryOnly(t *testing.T) {
	data := []byte(`{
		"summary": {
			"download_mbps": 321.4,
			"upload_mbps": 123.4,
			"latency_ms": 8.7,
			"server": "SG"
		}
	}`)

	got, err := parseSpeedtestJSON("speedtest_cn", data)
	if err != nil {
		t.Fatalf("parseSpeedtestJSON() error = %v", err)
	}
	if got.DownloadMbps != 321.4 || got.UploadMbps != 123.4 || got.LatencyMs != 8.7 {
		t.Fatalf("summary parse mismatch: %+v", got)
	}
	if got.Node != "SG" {
		t.Fatalf("Node = %q, want SG", got.Node)
	}
}

func TestPingWorkloadCachedRunReturnsCachedMeasurement(t *testing.T) {
	workload := &pingWorkload{
		detail:  "cached",
		elapsed: 123 * time.Millisecond,
		okCount: 7,
	}
	elapsed, processed, err := workload.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if elapsed != 123*time.Millisecond {
		t.Fatalf("elapsed = %s, want 123ms", elapsed)
	}
	if processed != 7 {
		t.Fatalf("processed = %d, want 7", processed)
	}
}
