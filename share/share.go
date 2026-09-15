// Package share projects a saved run or checkup report into a paste-friendly
// payload and uploads it to an explicitly chosen paste provider.
//
// Uploading is always an explicit user action (`vmbench share ...`): nothing
// in the benchmark, TUI, or MCP paths calls into this package. The payload is
// a faithful projection of the report — same data as JSON/HTML/markdown, no
// scores — and the default redaction mode masks the machine's public
// addresses and hostname before anything leaves the machine.
package share

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cloudapp3/vmbench/checkup"
	"github.com/cloudapp3/vmbench/history"
	"github.com/cloudapp3/vmbench/redact"
	"github.com/cloudapp3/vmbench/report"
	"github.com/cloudapp3/vmbench/sysinfo"
)

// Format selects the payload projection.
type Format string

const (
	// FormatText renders the same markdown projection as `--markdown`
	// (fenced textgrid tables, folded media) plus a provenance footer.
	FormatText Format = "text"
	// FormatJSON uploads the redacted report JSON verbatim, without footer.
	FormatJSON Format = "json"
)

// DefaultFormat is the format used when none is chosen.
const DefaultFormat = FormatText

// ParseFormat resolves a CLI format value; the empty string means the default.
func ParseFormat(value string) (Format, error) {
	switch Format(strings.ToLower(strings.TrimSpace(value))) {
	case "":
		return DefaultFormat, nil
	case FormatText:
		return FormatText, nil
	case FormatJSON:
		return FormatJSON, nil
	default:
		return "", fmt.Errorf("invalid share format %q (available: %s, %s)", value, FormatText, FormatJSON)
	}
}

// maxPayloadBytes is the upload size guard shared by every provider: an
// oversized payload fails loudly with convergence advice instead of being
// silently truncated. Provider-specific caps may tighten this further.
const maxPayloadBytes = 512 * 1024

// hostnameMask replaces the structured hostname field on the outbound copy.
// The report layer keeps hostnames (local-screen surfaces show them); share
// is the outbound surface, so it masks.
const hostnameMask = "hostname.redacted"

// Options selects the projection and redaction behavior of Prepare.
type Options struct {
	Format    Format      // zero value means DefaultFormat
	Redact    redact.Mode // zero value means redact.Default
	MediaFull bool        // text only: list every media service instead of the folded view
}

// Prepared is the upload-ready payload plus the redaction inventory.
type Prepared struct {
	Kind      history.Kind
	Payload   []byte
	Inventory Inventory
}

// Prepare detects the report kind, applies redaction, projects the payload,
// and enforces the size guard. It performs no network or filesystem access.
func Prepare(data []byte, opts Options) (Prepared, error) {
	meta, err := history.Inspect(data)
	if err != nil {
		return Prepared{}, &NotReportError{Err: err}
	}
	format := opts.Format
	if format == "" {
		format = DefaultFormat
	}
	inventory := Inventory{Mode: opts.Redact}
	if inventory.Mode == "" {
		inventory.Mode = redact.Default
	}

	prepared := Prepared{Kind: meta.Kind}
	switch meta.Kind {
	case history.KindRun:
		var doc report.Document
		if err := json.Unmarshal(data, &doc); err != nil {
			return Prepared{}, fmt.Errorf("share: decoding run report: %w", err)
		}
		if inventory.Mode.Enabled() {
			redacted, stats, err := redact.Apply(doc, nil)
			if err != nil {
				return Prepared{}, fmt.Errorf("share: redacting run report: %w", err)
			}
			doc = redacted
			inventory.fromStats(stats)
			inventory.HostnameMasked = maskHostname(&doc.System)
		}
		prepared.Inventory = inventory
		if format == FormatJSON {
			payload, err := marshalReportJSON(doc)
			if err != nil {
				return Prepared{}, fmt.Errorf("share: encoding run report: %w", err)
			}
			prepared.Payload = payload
		} else {
			var buf bytes.Buffer
			if err := report.WriteMarkdown(&buf, doc); err != nil {
				return Prepared{}, fmt.Errorf("share: rendering run report markdown: %w", err)
			}
			if err := appendFooter(&buf, inventory); err != nil {
				return Prepared{}, fmt.Errorf("share: rendering share footer: %w", err)
			}
			prepared.Payload = buf.Bytes()
		}
	case history.KindCheckup:
		var rep checkup.CheckupReport
		if err := json.Unmarshal(data, &rep); err != nil {
			return Prepared{}, fmt.Errorf("share: decoding checkup report: %w", err)
		}
		if inventory.Mode.Enabled() {
			redacted, stats, err := redact.Apply(rep, nil)
			if err != nil {
				return Prepared{}, fmt.Errorf("share: redacting checkup report: %w", err)
			}
			rep = redacted
			inventory.fromStats(stats)
			inventory.HostnameMasked = maskHostname(&rep.System)
		}
		prepared.Inventory = inventory
		if format == FormatJSON {
			payload, err := marshalReportJSON(rep)
			if err != nil {
				return Prepared{}, fmt.Errorf("share: encoding checkup report: %w", err)
			}
			prepared.Payload = payload
		} else {
			var buf bytes.Buffer
			if err := checkup.WriteMarkdownWithOptions(&buf, rep, checkup.MarkdownOptions{MediaFull: opts.MediaFull}); err != nil {
				return Prepared{}, fmt.Errorf("share: rendering checkup report markdown: %w", err)
			}
			if err := appendFooter(&buf, inventory); err != nil {
				return Prepared{}, fmt.Errorf("share: rendering share footer: %w", err)
			}
			prepared.Payload = buf.Bytes()
		}
	default:
		return Prepared{}, fmt.Errorf("share: unsupported report kind %q", meta.Kind)
	}

	if len(prepared.Payload) > maxPayloadBytes {
		return Prepared{}, &TooLargeError{Size: len(prepared.Payload), Limit: maxPayloadBytes}
	}
	return prepared, nil
}

// TooLargeError reports a payload above the upload guard; the CLI turns it
// into convergence advice (--format text, folded media) instead of uploading.
type TooLargeError struct {
	Size  int
	Limit int
}

func (e *TooLargeError) Error() string {
	return fmt.Sprintf("share: payload is %d bytes which exceeds the %d byte upload limit", e.Size, e.Limit)
}

// NotReportError reports input bytes that history.Inspect could not classify
// as a run or checkup report. The CLI treats it as a usage error (exit 2),
// unlike data-plane Prepare failures (exit 1).
type NotReportError struct {
	Err error
}

func (e *NotReportError) Error() string {
	return fmt.Sprintf("share: %s", e.Err.Error())
}

func (e *NotReportError) Unwrap() error { return e.Err }

// marshalReportJSON renders a report as indented JSON with a trailing
// newline, matching the CLI's `--json` byte shape.
func marshalReportJSON(v any) ([]byte, error) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// maskHostname replaces the structured hostname on the outbound copy and
// reports whether a value was masked. Free-text hostname occurrences are not
// scrubbed — the same scope the report-layer redaction applies to IPs.
func maskHostname(system *sysinfo.SystemInfo) int {
	if strings.TrimSpace(system.OS.Hostname) == "" {
		return 0
	}
	system.OS.Hostname = hostnameMask
	return 1
}
