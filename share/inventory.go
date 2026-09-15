package share

import (
	"github.com/cloudapp3/vmbench/i18n"
	"github.com/cloudapp3/vmbench/redact"
)

// Inventory records what redaction actually did to the payload. Counts only:
// the original addresses are never echoed back — printing what was just
// masked would undo the masking.
type Inventory struct {
	Mode           redact.Mode
	IPv4Count      int // distinct local IPv4 addresses masked
	IPv6Count      int // distinct local IPv6 addresses masked
	Sites          int // total replacement sites across all passes
	HostnameMasked int // 1 when the structured hostname was masked
}

// fromStats seeds the inventory from the report-layer redaction stats.
func (inv *Inventory) fromStats(stats redact.Stats) {
	inv.IPv4Count = len(stats.MappedIPv4)
	inv.IPv6Count = len(stats.MappedIPv6)
	inv.Sites = stats.Replacements
}

// Redacted reports whether anything was actually masked.
func (inv Inventory) Redacted() bool {
	return inv.IPv4Count > 0 || inv.IPv6Count > 0 || inv.HostnameMasked > 0
}

// Summary renders the localized one-line redaction summary. It is used both
// inside the text footer and on stderr after an upload, so the paste and the
// operator see the same claim — and the claim only ever states what really
// happened.
func (inv Inventory) Summary() string {
	if !inv.Mode.Enabled() {
		return i18n.T("cli.share.unredacted")
	}
	if !inv.Redacted() {
		return i18n.T("cli.share.redactedNothing")
	}
	return i18n.Tf("cli.share.redacted", map[string]any{
		"IPv4":     inv.IPv4Count,
		"IPv6":     inv.IPv6Count,
		"Hostname": inv.HostnameMasked,
		"Sites":    inv.Sites,
	})
}
