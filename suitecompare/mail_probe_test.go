package suitecompare

import (
	"fmt"
	"strings"
	"testing"
)

func TestCompareMailLatencyRequiresOpenConnection(t *testing.T) {
	report := func(status string, latency int) []byte {
		return []byte(fmt.Sprintf(`{
  "report_kind":"suite",
  "config":{},
  "mail":{"enabled":true,"status":"ok","results":[{
    "port":25,"title":"SMTP 25","target":"portquiz.net","method":"tcp_connect",
    "status":%q,"latency_ms":%d
  }]}
}`, status, latency))
	}

	t.Run("open latency is comparable", func(t *testing.T) {
		result, err := Compare([][]byte{report("open", 20), report("open", 10)})
		if err != nil {
			t.Fatal(err)
		}
		metric := findMetric(t, result, "mail", "25/latency")
		if !metric.Comparable || !strings.Contains(metric.Delta, "+50.0%") {
			t.Fatalf("open mail comparison = %+v", metric)
		}
	})

	for _, status := range []string{"refused", "timeout", "error"} {
		t.Run(status+" latency is unavailable", func(t *testing.T) {
			result, err := Compare([][]byte{report(status, 5000), report("open", 10)})
			if err != nil {
				t.Fatal(err)
			}
			metric := findMetric(t, result, "mail", "25/latency")
			if metric.Comparable || metric.Values[0].Available || !metric.Values[1].Available || !strings.Contains(metric.Reason, "missing") {
				t.Fatalf("%s mail comparison = %+v", status, metric)
			}
		})
	}
}

func TestCompareIPQualityPortLatencyRequiresOpenConnection(t *testing.T) {
	report := func(status string, latency int) []byte {
		return []byte(fmt.Sprintf(`{
  "report_kind":"suite",
  "config":{},
  "ip_quality":{"enabled":true,"status":"ok","result":{
    "port_25":{
      "port":25,"target":"portquiz.net","method":"tcp_connect",
      "status":%q,"latency_ms":%d
    },
    "mail_ports":[{
      "port":465,"target":"portquiz.net","method":"tcp_connect",
      "status":%q,"latency_ms":%d
    }]
  }}
}`, status, latency, status, latency))
	}

	metricNames := []string{
		"port_25/latency_ms",
		"mail_ports/port-465/latency_ms",
	}
	t.Run("open latency is comparable", func(t *testing.T) {
		result, err := Compare([][]byte{report("open", 20), report("open", 10)})
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range metricNames {
			metric := findMetric(t, result, "ip_quality", name)
			if !metric.Comparable || !strings.Contains(metric.Delta, "+50.0%") {
				t.Fatalf("open IP quality port comparison %s = %+v", name, metric)
			}
		}
	})

	for _, status := range []string{"refused", "timeout", "error"} {
		t.Run(status+" latency is unavailable", func(t *testing.T) {
			result, err := Compare([][]byte{report(status, 5000), report("open", 10)})
			if err != nil {
				t.Fatal(err)
			}
			for _, name := range metricNames {
				metric := findMetric(t, result, "ip_quality", name)
				if metric.Comparable || metric.Values[0].Available || !metric.Values[1].Available || !strings.Contains(metric.Reason, "missing") {
					t.Fatalf("%s IP quality port comparison %s = %+v", status, name, metric)
				}
			}
		})
	}
}

func TestCompareUnknownSectionDoesNotApplyIPQualityPortStatusGate(t *testing.T) {
	report := func(latency int) []byte {
		return []byte(fmt.Sprintf(`{
  "report_kind":"suite",
  "config":{},
  "future_probe":{"enabled":true,"result":{
    "port":443,"status":"ok","latency_ms":%d
  }}
}`, latency))
	}

	result, err := Compare([][]byte{report(20), report(10)})
	if err != nil {
		t.Fatal(err)
	}
	metric := findMetric(t, result, "future_probe", "latency_ms")
	if !metric.Comparable || !strings.Contains(metric.Delta, "+50.0%") {
		t.Fatalf("unknown-section port latency comparison = %+v", metric)
	}
}
