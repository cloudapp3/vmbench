package suitecompare

import "testing"

func TestCompareDoesNotTreatHTTPStatusAsContinuousMetric(t *testing.T) {
	report := []byte(`{
  "report_kind":"suite",
  "config":{},
  "reachability":{"enabled":true,"status":"ok","results":[
    {"id":"google","category":"website","protocol":"https","endpoint":"https://example.com","status":"reachable","latency_ms":12,"http_status":204}
  ]}
}`)
	result, err := Compare([][]byte{report, report})
	if err != nil {
		t.Fatalf("Compare() error = %v", err)
	}
	for _, metric := range result.Metrics {
		if metric.Section == "reachability" && metric.Name == "google/http_status" {
			t.Fatalf("HTTP status must remain categorical evidence, got metric %+v", metric)
		}
	}
}
