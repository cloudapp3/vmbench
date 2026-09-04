package netio

import (
	"context"
	"testing"
)

func TestHopASNIpv4CarrierRules(t *testing.T) {
	cases := map[string]string{
		"202.97.10.1":  "AS4134",
		"59.43.10.1":   "AS4809",
		"219.158.10.1": "AS4837",
		"218.105.10.1": "AS9929",
		"210.51.10.1":  "AS9929",
		"223.118.10.1": "AS58453",
		"223.118.34.1": "AS58807", // CMIN2 /21 exception inside AS58453 space
		"223.120.10.1": "AS58453",
		"221.183.10.1": "AS9808",
		"69.194.10.1":  "AS23764",
		"8.8.8.8":      "",
		"96.9.228.95":  "",
		"192.168.1.1":  "",
		"127.0.0.1":    "",
		"not-an-ip":    "",
	}
	for ip, want := range cases {
		if got := HopASN(ip); got != want {
			t.Errorf("HopASN(%q) = %q, want %q", ip, got, want)
		}
	}
}

func TestHopASNIpv6FromEmbeddedPrefixes(t *testing.T) {
	// 2400:9380:8001::/48 is listed in as4134.txt (电信 163 骨干网).
	if got := HopASN("2400:9380:8001::1"); got != "AS4134" {
		t.Errorf("HopASN ipv6 = %q, want AS4134", got)
	}
	// Non-carrier IPv6 must stay unlabeled.
	if got := HopASN("2001:4860:4860::8888"); got != "" {
		t.Errorf("HopASN non-carrier ipv6 = %q, want empty", got)
	}
}

func TestClassifyTraceResultsAnnotatesHopASNs(t *testing.T) {
	results := []TraceProbeResult{
		fixtureTraceResult("CT", "202.97.1.1", []string{"10.0.0.1", "202.97.10.1", "8.8.4.4"}),
	}
	ClassifyTraceResults(context.Background(), results)
	hops := results[0].Hops
	if hops[0].ASN != "" {
		t.Errorf("private hop annotated as %q, want empty", hops[0].ASN)
	}
	if hops[1].ASN != "AS4134" {
		t.Errorf("carrier hop annotated as %q, want AS4134", hops[1].ASN)
	}
	if hops[2].ASN != "" {
		t.Errorf("non-carrier hop annotated as %q, want empty", hops[2].ASN)
	}
}
