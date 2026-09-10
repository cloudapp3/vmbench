package checkup

import (
	"github.com/cloudapp3/vmbench/redact"
)

// redactReport masks the machine's public IP addresses in place. It is the
// single choke point for every checkup consumer: console, HTML, JSON,
// history, TUI, and MCP all read the report after Run returns. ModeNone is a
// no-op. On an unexpected Apply failure it fails closed — the network
// identity and IP quality evidence is dropped rather than returning a report
// with plaintext addresses.
func redactReport(report *CheckupReport, mode redact.Mode) {
	if !mode.Enabled() {
		return
	}
	redacted, _, err := redact.Apply(*report, nil)
	if err != nil {
		report.NetworkInfo.Result = nil
		report.IPQuality.Result = nil
		report.Warnings = append(report.Warnings, "public IP redaction failed: network identity and IP quality evidence suppressed ("+err.Error()+")")
		return
	}
	*report = redacted
}
