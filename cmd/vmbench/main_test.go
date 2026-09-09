package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunRejectsRemovedECSDiffCommands(t *testing.T) {
	for _, command := range []string{"ecs-diff", "ecs-compare"} {
		if code := run([]string{command}); code != 2 {
			t.Fatalf("run(%q) = %d, want unknown command exit 2", command, code)
		}
	}

	var output bytes.Buffer
	printUsage(&output)
	if strings.Contains(output.String(), "ecs-diff") || strings.Contains(output.String(), "ecs-compare") {
		t.Fatalf("usage still advertises removed ECS command:\n%s", output.String())
	}
}

func TestRemovedRunSuiteCommandsReportMigration(t *testing.T) {
	// "run" and "suite" are the removed pre-v0.8.0 subcommand names.
	for _, command := range []string{"run", "suite"} {
		output, code := captureStderr(t, func() int { return run([]string{command}) })
		if code != 2 {
			t.Fatalf("run(%q) = %d, want migration exit 2", command, code)
		}
		for _, want := range []string{"v0.8.0", "vmbench --help"} {
			if !strings.Contains(output, want) {
				t.Errorf("run(%q) stderr missing %q:\n%s", command, want, output)
			}
		}
	}
}

func TestRunRejectsInvalidBenchmarkArguments(t *testing.T) {
	tests := [][]string{
		{"--iterations", "0"},
		{"--iterations", "10"},
		{"--filter", "["},
		{"--mode", "parallel"},
		{"--scope", "internet"},
		{"--hardware-tool", "openssl,unknown"},
		{"--history-tag", "missing-save-flag"},
		{"--json", "out.json", "stray-positional"},
		{"--only", "hardware", "--speed-provider", "cloudflare,unknown"},
	}
	for _, args := range tests {
		if code := run(args); code != 2 {
			t.Fatalf("run(%v) = %d, want 2", args, code)
		}
	}
}

func TestRunRejectsInvalidSectionArguments(t *testing.T) {
	tests := [][]string{
		{"--ip-version", "v5"},
		{"--only", "hardware,unknown"},
		{"--skip", "media,unknown"},
	}
	for _, args := range tests {
		if code := run(args); code != 2 {
			t.Fatalf("run(%v) = %d, want 2", args, code)
		}
	}
}

func TestInvalidSectionNameResolvesAliases(t *testing.T) {
	for _, value := range []string{"network_info", "network-identity", "netinfo", "reachability", "website", "telegram"} {
		if got := invalidSectionName(value); got != "" {
			t.Fatalf("invalidSectionName(%q) = %q, want accepted", value, got)
		}
	}
	if got := invalidSectionName("bogus"); got != "bogus" {
		t.Fatalf("invalidSectionName(bogus) = %q, want bogus", got)
	}
}

func TestSectionAliasesAcceptedThroughRootFlags(t *testing.T) {
	// The revision mismatch is intentional: it proves the alias resolves and
	// the run reaches catalog preflight instead of failing section parsing.
	for _, only := range []string{"network", "latency", "traceroute"} {
		args := []string{"--only", only, "--node-revision", "missing-revision"}
		if code := run(args); code != 2 {
			t.Fatalf("run(%v) = %d, want catalog preflight exit 2", args, code)
		}
	}
}

func TestRunIperfWithoutHostExitsNonZero(t *testing.T) {
	if code := run([]string{"--only", "speed", "--speed-provider", "iperf3"}); code != 1 {
		t.Fatalf("run(iperf3 without host) = %d, want 1", code)
	}
}

func TestRunRejectsPinnedCatalogMismatchBeforeNetworkExecution(t *testing.T) {
	tests := [][]string{
		{"--only", "ping", "--node-revision", "missing-revision"},
	}
	for _, args := range tests {
		if code := run(args); code != 2 {
			t.Fatalf("run(%v) = %d, want configuration error 2", args, code)
		}
	}
}

func TestAcceptsExpandedRoutePresetsWithoutStartingNetwork(t *testing.T) {
	// The revision mismatch is intentional: it proves every new preset passes
	// parsing and reaches catalog preflight without executing probes.
	if code := run([]string{
		"--only", "route", "--route-presets", "cd,cernet,cstnet",
		"--ip-version", "dual", "--node-revision", "missing-revision",
	}); code != 2 {
		t.Fatalf("expanded route preset preflight code = %d, want 2", code)
	}
}
