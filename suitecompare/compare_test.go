package suitecompare

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func TestCompareCompatibleRawMetricsAndUnknownSection(t *testing.T) {
	base := suiteFixture("catalog-1", "ping.example", "cloudflare", "speed.cloudflare.com", 20, 800, 7)
	target := suiteFixture("catalog-1", "ping.example", "cloudflare", "speed.cloudflare.com", 10, 1000, 9)
	result, err := Compare([][]byte{base, target})
	if err != nil {
		t.Fatal(err)
	}
	latency := findMetric(t, result, "ping", "node-a/latency")
	if !latency.Comparable || !strings.Contains(latency.Delta, "▲+50.0%") {
		t.Fatalf("ping comparison = %+v", latency)
	}
	download := findMetric(t, result, "speed", "cf-download/download")
	if !download.Comparable || !strings.Contains(download.Delta, "▲+25.0%") {
		t.Fatalf("speed comparison = %+v", download)
	}
	queue := findMetric(t, result, "future_probe", "queue_depth")
	if !queue.Comparable || queue.Delta != "+28.6%" {
		t.Fatalf("unknown section comparison = %+v", queue)
	}
}

func TestCompareRejectsIncompatibleDimensions(t *testing.T) {
	tests := []struct {
		name   string
		base   []byte
		target []byte
		metric string
		reason string
	}{
		{name: "catalog", base: suiteFixture("catalog-1", "ping.example", "cloudflare", "speed.cloudflare.com", 20, 800, 7), target: suiteFixture("catalog-2", "ping.example", "cloudflare", "speed.cloudflare.com", 10, 1000, 9), metric: "node-a/latency", reason: "catalog revision differs"},
		{name: "target", base: suiteFixture("catalog-1", "ping-a.example", "cloudflare", "speed.cloudflare.com", 20, 800, 7), target: suiteFixture("catalog-1", "ping-b.example", "cloudflare", "speed.cloudflare.com", 10, 1000, 9), metric: "node-a/latency", reason: "node/target differs"},
		{name: "protocol", base: suiteFixtureWithPingProtocol("catalog-1", "ping.example", "tcp", "cloudflare", "speed.cloudflare.com", 20, 800, 7), target: suiteFixtureWithPingProtocol("catalog-1", "ping.example", "udp", "cloudflare", "speed.cloudflare.com", 10, 1000, 9), metric: "node-a/latency", reason: "protocol differs"},
		{name: "provider", base: suiteFixture("catalog-1", "ping.example", "cloudflare", "speed.cloudflare.com", 20, 800, 7), target: suiteFixture("catalog-1", "ping.example", "other", "speed.cloudflare.com", 10, 1000, 9), metric: "cf-download/download", reason: "provider differs"},
		{name: "speed node", base: suiteFixture("catalog-1", "ping.example", "cloudflare", "node-a", 20, 800, 7), target: suiteFixture("catalog-1", "ping.example", "cloudflare", "node-b", 10, 1000, 9), metric: "cf-download/download", reason: "node/target differs"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Compare([][]byte{tt.base, tt.target})
			if err != nil {
				t.Fatal(err)
			}
			section := "ping"
			if strings.Contains(tt.metric, "download") {
				section = "speed"
			}
			metric := findMetric(t, result, section, tt.metric)
			if metric.Comparable || metric.Delta != "N/A" || !strings.Contains(metric.Reason, tt.reason) {
				t.Fatalf("comparison = %+v, want reason %q", metric, tt.reason)
			}
			if len(metric.Values) != 2 || !metric.Values[0].Available || !metric.Values[1].Available {
				t.Fatalf("incompatible comparison discarded raw values: %+v", metric.Values)
			}
		})
	}
}

func TestCompareRequiresCatalogRevisionForNodeMetrics(t *testing.T) {
	base := suiteFixture("", "ping.example", "cloudflare", "speed.cloudflare.com", 20, 800, 7)
	target := suiteFixture("", "ping.example", "cloudflare", "speed.cloudflare.com", 10, 1000, 9)
	result, err := Compare([][]byte{base, target})
	if err != nil {
		t.Fatal(err)
	}
	metric := findMetric(t, result, "ping", "node-a/latency")
	if metric.Comparable || !strings.Contains(metric.Reason, "catalog revision is unknown") {
		t.Fatalf("comparison = %+v", metric)
	}
	unknown := findMetric(t, result, "future_probe", "queue_depth")
	if !unknown.Comparable {
		t.Fatalf("non-node unknown section should remain extensible: %+v", unknown)
	}
}

func TestCompareHardwareThroughputRequiresMatchingUnit(t *testing.T) {
	base := hardwareFixture("ops/s", 100)
	target := hardwareFixture("MB/s", 200)
	result, err := Compare([][]byte{base, target})
	if err != nil {
		t.Fatal(err)
	}
	metric := findMetric(t, result, "hardware", "OpenSSL AES/throughput")
	if metric.Comparable || !strings.Contains(metric.Reason, "unit differs") || metric.Delta != "N/A" {
		t.Fatalf("comparison = %+v", metric)
	}
}

func TestCompareGenericSectionRequiresMatchingProtocolAndExplicitUnit(t *testing.T) {
	report := func(protocol, unit string, value int) []byte {
		return []byte(fmt.Sprintf(`{
  "report_kind":"suite",
  "config":{},
  "future":{"enabled":true,"result":{"protocol":%q,"provider":"probe","target":"target-a","unit":%q,"value":%d}}
}`, protocol, unit, value))
	}
	tests := []struct {
		name   string
		base   []byte
		target []byte
		reason string
	}{
		{name: "protocol", base: report("udp", "requests/s", 10), target: report("tcp", "requests/s", 20), reason: "protocol differs"},
		{name: "explicit unit", base: report("udp", "requests/s", 10), target: report("udp", "bytes/s", 20), reason: "unit differs"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Compare([][]byte{tt.base, tt.target})
			if err != nil {
				t.Fatal(err)
			}
			metric := findMetric(t, result, "future", "value")
			if metric.Comparable || metric.Delta != "N/A" || !strings.Contains(metric.Reason, tt.reason) {
				t.Fatalf("comparison = %+v", metric)
			}
		})
	}
}

func TestCompareReachabilityRequiresMatchingProtocolAndEndpoint(t *testing.T) {
	report := func(protocol, endpoint string, latency int) []byte {
		return []byte(fmt.Sprintf(`{
  "report_kind":"suite",
  "config":{},
  "reachability":{"enabled":true,"status":"ok","results":[
    {"id":"telegram_dc1","category":"telegram","protocol":%q,"endpoint":%q,"status":"reachable","latency_ms":%d}
  ]}
}`, protocol, endpoint, latency))
	}
	base := report("tcp", "149.154.175.53:443", 20)
	target := report("tcp", "149.154.175.53:443", 10)
	result, err := Compare([][]byte{base, target})
	if err != nil {
		t.Fatal(err)
	}
	metric := findMetric(t, result, "reachability", "telegram_dc1/latency_ms")
	if !metric.Comparable || !strings.Contains(metric.Delta, "+50.0%") {
		t.Fatalf("reachability comparison = %+v", metric)
	}

	result, err = Compare([][]byte{base, report("https", "https://example.com", 10)})
	if err != nil {
		t.Fatal(err)
	}
	metric = findMetric(t, result, "reachability", "telegram_dc1/latency_ms")
	if metric.Comparable || metric.Delta != "N/A" || (!strings.Contains(metric.Reason, "protocol differs") && !strings.Contains(metric.Reason, "node/target differs")) {
		t.Fatalf("incompatible reachability comparison = %+v", metric)
	}
}

func TestWriteCompareAndRejectNonSuite(t *testing.T) {
	base := suiteFixture("catalog-1", "ping.example", "cloudflare", "speed.cloudflare.com", 20, 800, 7)
	var output bytes.Buffer
	if err := WriteCompare(&output, [][]byte{base, base}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "VMBench Suite Compare") || !strings.Contains(output.String(), "Raw Metrics") {
		t.Fatalf("output = %s", output.String())
	}
	if _, err := Compare([][]byte{[]byte(`{"results":{"workloads":[]}}`), base}); err == nil {
		t.Fatal("Compare accepted a run report")
	}
}

func findMetric(t *testing.T, result Result, section, name string) MetricComparison {
	t.Helper()
	for _, metric := range result.Metrics {
		if metric.Section == section && metric.Name == name {
			return metric
		}
	}
	t.Fatalf("metric %s/%s not found in %+v", section, name, result.Metrics)
	return MetricComparison{}
}

func suiteFixture(revision, pingTarget, speedProvider, speedNode string, latency, download, queue float64) []byte {
	return suiteFixtureWithPingProtocol(revision, pingTarget, "tcp", speedProvider, speedNode, latency, download, queue)
}

func suiteFixtureWithPingProtocol(revision, pingTarget, pingProtocol, speedProvider, speedNode string, latency, download, queue float64) []byte {
	return []byte(`{
  "schema_version": 2,
  "report_kind": "suite",
  "report_id": "fixture",
  "started_at": "2026-07-13T01:00:00Z",
  "catalog_revision": "` + revision + `",
	  "config": {},
	  "system": {"cpu": {"model": "Fixture CPU"}},
	  "ping": {"enabled": true, "status": "ok", "results": [
	    {"id": "node-a", "target": "` + pingTarget + `", "ip_family": "v4", "protocol": "` + pingProtocol + `", "status": "ok", "avg_latency_ms": ` + number(latency) + `}
  ]},
  "speed": {"enabled": true, "status": "ok", "result": {"providers": [
    {"id": "cf-download", "provider": "` + speedProvider + `", "status": "ok", "node": "` + speedNode + `", "download_mbps": ` + number(download) + `}
  ]}},
  "future_probe": {"enabled": true, "status": "ok", "result": {
    "protocol": "udp", "provider": "future", "target": "future.example", "queue_depth": ` + number(queue) + `
  }}
}`)
}

func hardwareFixture(unit string, throughput float64) []byte {
	return []byte(`{
  "version": 1,
  "config": {},
  "hardware": {"enabled": true, "status": "ok", "report": {"results": {"workloads": [
    {"name": "OpenSSL AES", "category": "cpu", "result": {"median_ms": 10, "throughput_per_sec": ` + number(throughput) + `, "throughput_unit": "` + unit + `"}}
  ]}}}
}`)
}

func number(value float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.6f", value), "0"), ".")
}
