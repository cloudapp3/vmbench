package suitecompare

import (
	"fmt"
	"strings"
	"testing"
)

func TestComparePingRequiresMatchingPort(t *testing.T) {
	report := func(port int) []byte {
		return []byte(fmt.Sprintf(`{
  "report_kind":"suite","config":{"catalog_revision":"catalog-1"},
  "ping":{"enabled":true,"status":"ok","results":[{
    "id":"ping-1","target":"ping.example","port":%d,"ip_family":"v4","source":"catalog",
    "probe_protocol":"tcp-connect","probe_tool":"go-net-dialer","status":"ok","avg_latency_ms":10
  }]}
}`, port))
	}
	result, err := Compare([][]byte{report(80), report(443)})
	if err != nil {
		t.Fatalf("Compare() error = %v", err)
	}
	metric := findMetric(t, result, "ping", "ping-1/latency")
	if metric.Comparable || !strings.Contains(metric.Reason, "node/target differs") {
		t.Fatalf("comparison = %+v", metric)
	}
}

func TestCompareSpeedRequiresMatchingStableEndpoint(t *testing.T) {
	report := func(endpoint string) []byte {
		return []byte(fmt.Sprintf(`{
  "report_kind":"suite","config":{"catalog_revision":"catalog-1"},
  "speed":{"enabled":true,"status":"ok","result":{"providers":[{
    "id":"speedtest-net","provider":"speedtest_net","status":"ok","node_id":"12345","endpoint":%q,"node":"Example ISP","download_mbps":100
  }]}}
}`, endpoint))
	}
	result, err := Compare([][]byte{report("one.example:8080"), report("two.example:8080")})
	if err != nil {
		t.Fatalf("Compare() error = %v", err)
	}
	metric := findMetric(t, result, "speed", "speedtest-net/download")
	if metric.Comparable || !strings.Contains(metric.Reason, "node/target differs") {
		t.Fatalf("comparison = %+v", metric)
	}
}
