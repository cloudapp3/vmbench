package vmbench

import (
	"github.com/cloudapp3/vmbench/redact"
)

// redactDocument masks the machine's public IPs in a run report document and
// returns warnings to attach. Run reports carry no identity fields today, so
// the pass is dormant; it exists so any future workload output that embeds
// addresses lands redacted in every consumer (JSON, HTML, history, TUI
// export, MCP) without extra wiring.
func redactDocument(document Report, mode redact.Mode) (Report, []string) {
	if !mode.Enabled() {
		return document, nil
	}
	redacted, _, err := redact.Apply(document, nil)
	if err != nil {
		// No identity fields exist to suppress; surface the failure and keep
		// the document (Apply only errors on JSON round-trip bugs, which the
		// redact package tests lock down).
		return document, []string{"public IP redaction failed: " + err.Error()}
	}
	return redacted, nil
}
