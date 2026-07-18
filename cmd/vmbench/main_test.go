package main

import "testing"

func TestRunRejectsInvalidBenchmarkArguments(t *testing.T) {
	tests := [][]string{
		{"run", "--iterations", "0"},
		{"run", "--filter", "["},
		{"run", "--mode", "parallel"},
		{"run", "--scope", "internet"},
		{"run", "--hardware-tool", "openssl,unknown"},
		{"run", "--history-tag", "missing-save-flag"},
	}
	for _, args := range tests {
		if code := run(args); code != 2 {
			t.Fatalf("run(%v) = %d, want 2", args, code)
		}
	}
}

func TestRunRejectsInvalidSuiteArguments(t *testing.T) {
	tests := [][]string{
		{"suite", "--iterations", "10"},
		{"suite", "--filter", "["},
		{"suite", "--ip-version", "v5"},
		{"suite", "--only", "hardware,unknown"},
		{"suite", "--speed-provider", "cloudflare,unknown"},
		{"suite", "--history-tag", "missing-save-flag"},
	}
	for _, args := range tests {
		if code := run(args); code != 2 {
			t.Fatalf("run(%v) = %d, want 2", args, code)
		}
	}
}

func TestRunSuiteIperfWithoutHostExitsNonZero(t *testing.T) {
	if code := run([]string{"suite", "--only", "speed", "--speed-provider", "iperf3"}); code != 1 {
		t.Fatalf("run(suite iperf3 without host) = %d, want 1", code)
	}
}

func TestRunRejectsPinnedCatalogMismatchBeforeNetworkExecution(t *testing.T) {
	tests := [][]string{
		{"run", "--scope", "network", "--node-revision", "missing-revision"},
		{"suite", "--only", "ping", "--node-revision", "missing-revision"},
	}
	for _, args := range tests {
		if code := run(args); code != 2 {
			t.Fatalf("run(%v) = %d, want configuration error 2", args, code)
		}
	}
}

func TestSuiteAcceptsExpandedRoutePresetsWithoutStartingNetwork(t *testing.T) {
	// The revision mismatch is intentional: it proves every new preset passes
	// parsing and reaches catalog preflight without executing probes.
	if code := run([]string{
		"suite", "--only", "route", "--route-presets", "cd,cernet,cstnet",
		"--ip-version", "dual", "--node-revision", "missing-revision",
	}); code != 2 {
		t.Fatalf("expanded route preset preflight code = %d, want 2", code)
	}
}

func TestInvalidSectionNameAcceptsNetworkEvidenceAliases(t *testing.T) {
	for _, value := range []string{"network_info", "network-identity", "netinfo", "reachability", "website", "telegram"} {
		if got := invalidSectionName(value); got != "" {
			t.Fatalf("invalidSectionName(%q) = %q, want accepted", value, got)
		}
	}
}
