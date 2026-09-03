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
	"resolved_target":"203.0.113.8","destination_reached":true,"status":"ok",
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

func TestCompareRouteSkipsPartialDestinationAndUsesResolvedTarget(t *testing.T) {
	report := func(status string, reached bool, resolved string) []byte {
		return []byte(fmt.Sprintf(`{
  "report_kind":"suite",
  "config":{"catalog_revision":"catalog-1"},
  "route":{"enabled":true,"status":%q,"results":[{
    "target":{"id":"route-1","name":"Route","ip_family":"v4","protocol":"tcp","endpoint":"route.example","source":"catalog"},
    "resolved_target":%q,"destination_reached":%t,"status":%q,
    "probe_protocol":"tcp","probe_tool":"traceroute","hops":[{"ttl":1,"ip":"192.0.2.1","rtt_ms":10}]
  }]}
}`, status, resolved, reached, status))
	}

	t.Run("partial route is not comparable as a completed route", func(t *testing.T) {
		result, err := Compare([][]byte{
			report("partial", false, "203.0.113.8"),
			report("ok", true, "203.0.113.8"),
		})
		if err != nil {
			t.Fatal(err)
		}
		metric := findMetric(t, result, "route", "route-1/hop count")
		if metric.Comparable || metric.Values[0].Available || !metric.Values[1].Available || !strings.Contains(metric.Reason, "missing") {
			t.Fatalf("partial route comparison = %+v", metric)
		}
	})

	t.Run("resolved target is a compatibility dimension", func(t *testing.T) {
		result, err := Compare([][]byte{
			report("ok", true, "203.0.113.8"),
			report("ok", true, "203.0.113.9"),
		})
		if err != nil {
			t.Fatal(err)
		}
		metric := findMetric(t, result, "route", "route-1/hop count")
		if metric.Comparable || !strings.Contains(metric.Reason, "node/target differs") {
			t.Fatalf("resolved-target comparison = %+v", metric)
		}
	})

	t.Run("legacy route without destination evidence is unavailable", func(t *testing.T) {
		legacy := []byte(`{
  "report_kind":"suite",
  "config":{"catalog_revision":"catalog-1"},
  "route":{"enabled":true,"status":"ok","results":[{
    "target":{"id":"route-1","name":"Route","ip_family":"v4","protocol":"tcp","endpoint":"route.example","source":"catalog"},
    "probe_protocol":"tcp","probe_tool":"traceroute","hops":[{"ttl":1,"ip":"192.0.2.1","rtt_ms":10}]
  }]}
}`)
		result, err := Compare([][]byte{
			legacy,
			report("ok", true, "203.0.113.8"),
		})
		if err != nil {
			t.Fatal(err)
		}
		metric := findMetric(t, result, "route", "route-1/hop count")
		if metric.Comparable || metric.Values[0].Available || !metric.Values[1].Available || !strings.Contains(metric.Reason, "missing") {
			t.Fatalf("legacy route comparison = %+v", metric)
		}
	})
}
