package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/cloudapp3/vmbench"
	"github.com/cloudapp3/vmbench/i18n"
	"github.com/cloudapp3/vmbench/sysinfo"
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

func TestCompareSurfacesHiddenWhileFeatureCompareOff(t *testing.T) {
	if vmbench.FeatureCompare {
		t.Skip("compare surfaces exposed (FeatureCompare=true)")
	}
	for _, argv := range [][]string{
		{"compare", "a.json", "b.json"},
		{"history", "compare", "--last", "2"},
	} {
		if code := run(argv); code != 2 {
			t.Fatalf("run(%v) = %d, want unknown command exit 2", argv, code)
		}
	}

	var output bytes.Buffer
	printUsage(&output)
	if strings.Contains(output.String(), "vmbench compare") {
		t.Fatalf("usage still advertises hidden compare command:\n%s", output.String())
	}
	printHistoryUsage(&output)
	if strings.Contains(output.String(), "history compare") {
		t.Fatalf("history usage still advertises hidden compare subcommand:\n%s", output.String())
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

func TestRedactFlagValidation(t *testing.T) {
	withLangEnv(t, "en")
	if code := run([]string{"--redact", "bogus"}); code != 2 {
		t.Fatalf("run(--redact bogus) = %d, want 2", code)
	}
	// --redact none passes its own check and prints the share warning before
	// the later --only failure aborts the run.
	output, code := captureStderr(t, func() int { return run([]string{"--redact", "none", "--only", "unknown"}) })
	if code != 2 {
		t.Fatalf("run(--redact none --only unknown) = %d, want 2", code)
	}
	if !strings.Contains(output, "--redact none keeps real public IPs") {
		t.Errorf("stderr missing the none notice:\n%s", output)
	}
	// Mode values are case-insensitive.
	output, code = captureStderr(t, func() int { return run([]string{"--redact", "IPS", "--only", "unknown"}) })
	if code != 2 {
		t.Fatalf("run(--redact IPS --only unknown) = %d, want 2", code)
	}
	if strings.Contains(output, "unknown redact mode") {
		t.Errorf("--redact IPS should validate:\n%s", output)
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

func TestWriteSysinfoConsoleShowsDMIAndMemorySpec(t *testing.T) {
	info := sysinfo.SystemInfo{
		OS:  sysinfo.OSInfo{Name: "Debian GNU/Linux 12", Kernel: "6.1", Hostname: "vps"},
		CPU: sysinfo.CPUInfo{Model: "EPYC", Arch: "amd64", PhysicalCores: 2, LogicalCores: 4},
		Memory: sysinfo.MemoryInfo{
			TotalBytes: 16 << 30, Type: "DDR4", FreqMHz: 2666, Channels: 2,
		},
		Virtualization: sysinfo.VirtualizationInfo{System: "kvm", Role: "guest"},
		DMI:            sysinfo.DMIInfo{ProductName: "Alibaba Cloud ECS", SysVendor: "Alibaba Cloud"},
	}
	var out bytes.Buffer
	writeSysinfoConsole(&out, info, nil)
	text := out.String()
	for _, want := range []string{
		"kvm (guest)",
		"Alibaba Cloud ECS (Alibaba Cloud)",
		"16.0 GB DDR4 2666 MT/s ×2",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("sysinfo console missing %q:\n%s", want, text)
		}
	}
}

func TestWriteSysinfoConsoleOmitsUnknownMemoryAndDMI(t *testing.T) {
	var out bytes.Buffer
	writeSysinfoConsole(&out, sysinfo.SystemInfo{}, nil)
	text := out.String()
	for _, banned := range []string{"MT/s", "×", i18n.T("cli.sysinfo.dmi")} {
		if strings.Contains(text, banned) {
			t.Fatalf("sysinfo console must omit unknown DMI/memory evidence, found %q:\n%s", banned, text)
		}
	}
}

func TestAutoSwapFlagParses(t *testing.T) {
	// The revision mismatch is intentional: it proves --auto-swap parses and
	// the run reaches catalog preflight without executing probes.
	if code := run([]string{"--only", "ping", "--auto-swap", "--node-revision", "missing-revision"}); code != 2 {
		t.Fatalf("run(--auto-swap) = %d, want catalog preflight exit 2", code)
	}
}
