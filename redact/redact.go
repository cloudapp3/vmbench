// Package redact masks the local machine's public IP addresses in benchmark
// reports. Redaction runs once, at report assembly, so every consumer
// (console, HTML, JSON, history, TUI, MCP) sees the same masked values.
//
// Addresses are replaced with placeholders from the documentation ranges:
// IPv4 → 203.0.113.x (RFC 5737 TEST-NET-3), IPv6 → 2001:db8::x (RFC 3849).
// The mapping is stable within one report, so cross-references between the
// structured identity fields, free-text evidence (securityCheck raw output,
// DNSBL messages, error strings), and embedded documents stay coherent.
package redact

import (
	"encoding/json"
	"fmt"
	"net"
	"strings"
)

// Mode selects the report redaction behavior.
type Mode string

const (
	// ModeIPs masks the machine's public IPv4/IPv6 addresses. It is the
	// default: reports are redacted unless the caller explicitly opts out.
	ModeIPs Mode = "ips"
	// ModeNone disables redaction entirely.
	ModeNone Mode = "none"
)

// Default is the mode applied when no explicit choice was made.
const Default Mode = ModeIPs

// Parse resolves a CLI/MCP redact value; the empty string means Default.
// Further values (strict) are reserved for the planned share subcommand.
func Parse(value string) (Mode, error) {
	switch Mode(strings.ToLower(strings.TrimSpace(value))) {
	case "":
		return Default, nil
	case ModeIPs:
		return ModeIPs, nil
	case ModeNone:
		return ModeNone, nil
	default:
		return "", fmt.Errorf("invalid redact mode %q (available: %s, %s)", value, ModeIPs, ModeNone)
	}
}

// Enabled reports whether the mode masks public IP addresses.
func (m Mode) Enabled() bool { return m != ModeNone }

// Stats records what Apply mapped, for tests and the future share inventory.
type Stats struct {
	MappedIPv4   []string // original addresses, in assignment order
	MappedIPv6   []string
	Replacements int // total substitution sites across all passes
}

// Apply returns a redacted copy of report. The machine's public addresses
// are harvested from the report's identity fields unless locals is non-nil.
// When no redactable address is found the report value is returned unchanged
// without a JSON round trip.
func Apply[T any](report T, locals []net.IP) (T, Stats, error) {
	var stats Stats
	raw, err := json.Marshal(report)
	if err != nil {
		return report, stats, fmt.Errorf("redact: marshal report: %w", err)
	}
	addresses := locals
	if addresses == nil {
		addresses = harvestPublicIPs(raw)
	}
	mapping := newMapping(addresses)
	if len(mapping.entries) == 0 {
		return report, stats, nil
	}
	stats.MappedIPv4, stats.MappedIPv6 = mapping.mappedLists()
	redacted, replacements := mapping.replaceAll(raw)
	var out T
	if err := json.Unmarshal(redacted, &out); err != nil {
		return report, stats, fmt.Errorf("redact: unmarshal redacted report: %w", err)
	}
	stats.Replacements = replacements
	return out, stats, nil
}
