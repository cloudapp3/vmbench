package netio

import "testing"

func TestParseSpeedtestJSONKeepsStableServerIdentity(t *testing.T) {
	result, err := parseSpeedtestJSON("speedtest_net", []byte(`{
  "download":{"bandwidth":1250000},
  "server":{"id":12345,"host":"speed.example","port":8080,"name":"Example ISP","location":"Tokyo"}
}`))
	if err != nil {
		t.Fatalf("parseSpeedtestJSON() error = %v", err)
	}
	if result.NodeID != "12345" || result.Endpoint != "speed.example:8080" || result.Node != "Example ISP" {
		t.Fatalf("server identity = %+v", result)
	}
}
