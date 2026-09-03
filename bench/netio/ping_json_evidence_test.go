package netio

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPingResultJSONRetainsSuccessfulZeroMetrics(t *testing.T) {
	data, err := json.Marshal(PingProbeResult{
		ID:              "zero-loss",
		Status:          "ok",
		ConnectionState: PingConnectionStateRefused,
		Sent:            10,
		Received:        10,
	})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	text := string(data)
	for _, field := range []string{`"connection_state":"refused"`, `"avg_latency_ms":0`, `"jitter_ms":0`, `"packet_loss":0`} {
		if !strings.Contains(text, field) {
			t.Fatalf("JSON = %s, missing raw metric %s", text, field)
		}
	}
}
