package vmbench

import (
	"strings"
	"testing"
)

func TestPrepareOptionsUsesSafeDefaults(t *testing.T) {
	norm, filter, warnings := prepareOptions(Options{})
	if norm.Scope != ScopeHardware {
		t.Fatalf("scope = %q, want %q", norm.Scope, ScopeHardware)
	}
	if norm.Mode != "single" {
		t.Fatalf("mode = %q, want single", norm.Mode)
	}
	if filter != "" || len(warnings) != 0 {
		t.Fatalf("filter=%q warnings=%v, want empty", filter, warnings)
	}
}

func TestPrepareOptionsLegacyModeRunsCatalogOnce(t *testing.T) {
	norm, _, warnings := prepareOptions(Options{Mode: "all"})
	if norm.Mode != "single" {
		t.Fatalf("mode = %q, want single", norm.Mode)
	}
	if len(warnings) == 0 || !strings.Contains(warnings[0], "legacy mode all") {
		t.Fatalf("warnings = %v, want legacy mode warning", warnings)
	}
}

func TestPrepareOptionsInvalidFilterSelectsNothing(t *testing.T) {
	_, filter, warnings := prepareOptions(Options{Filter: "["})
	if filter != "a^" {
		t.Fatalf("filter = %q, want never-match expression", filter)
	}
	if len(warnings) == 0 || !strings.Contains(warnings[0], "no workloads selected") {
		t.Fatalf("warnings = %v, want invalid filter warning", warnings)
	}
}

func TestPrepareOptionsNetworkScopeDoesNotSelectHardwareTools(t *testing.T) {
	norm, _, _ := prepareOptions(Options{Scope: ScopeNetwork})
	if len(norm.HardwareTools) != 0 {
		t.Fatalf("hardware tools = %v, want none", norm.HardwareTools)
	}
}

func TestPrepareOptionsKeepsOnlyUsedIperfHosts(t *testing.T) {
	hardware, _, _ := prepareOptions(Options{Scope: ScopeHardware, IperfHosts: []string{"unused.example:5201"}})
	if len(hardware.IperfHosts) != 0 {
		t.Fatalf("hardware iperf hosts = %v, want none", hardware.IperfHosts)
	}

	network, _, _ := prepareOptions(Options{Scope: ScopeNetwork, IperfHosts: []string{" one.example:5201 ", "", "one.example:5201"}})
	if len(network.IperfHosts) != 1 || network.IperfHosts[0] != "one.example:5201" {
		t.Fatalf("network iperf hosts = %v, want one normalized host", network.IperfHosts)
	}
}
