package suitecompare

import (
	"fmt"
	"strings"
	"testing"
)

func TestCompareRouteRequiresActualProbeProtocolToolAndFamily(t *testing.T) {
	report := func(protocol, tool, family string) []byte {
		return []byte(fmt.Sprintf(`{
  "report_kind":"suite",
  "config":{"catalog_revision":"catalog-1"},
  "route":{"enabled":true,"status":"ok","results":[{
    "target":{"id":"route-1","name":"Route","ip_family":%q,"protocol":"tcp","endpoint":"route.example","source":"catalog"},
    "probe_protocol":%q,"probe_tool":%q,"hops":[{"ttl":1,"ip":"192.0.2.1","rtt_ms":10}]
  }]}
}`, family, protocol, tool))
	}
	tests := []struct {
		name   string
		other  []byte
		reason string
	}{
		{name: "protocol", other: report("udp", "traceroute", "v4"), reason: "protocol differs"},
		{name: "tool", other: report("tcp", "tracepath", "v4"), reason: "provider differs"},
		{name: "family", other: report("tcp", "traceroute", "v6"), reason: "protocol differs"},
	}
	base := report("tcp", "traceroute", "v4")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Compare([][]byte{base, tt.other})
			if err != nil {
				t.Fatalf("Compare() error = %v", err)
			}
			metric := findMetric(t, result, "route", "route-1/hop count")
			if metric.Comparable || metric.Delta != "N/A" || !strings.Contains(metric.Reason, tt.reason) {
				t.Fatalf("comparison = %+v", metric)
			}
		})
	}
}
