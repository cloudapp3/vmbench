package share

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/cloudapp3/vmbench/checkup"
	"github.com/cloudapp3/vmbench/history"
	"github.com/cloudapp3/vmbench/i18n"
	"github.com/cloudapp3/vmbench/redact"
	"github.com/cloudapp3/vmbench/report"
	"github.com/cloudapp3/vmbench/sysinfo"
)

// shareCheckupData builds a checkup report carrying the share-relevant
// shapes: identity fields with public addresses (the harvest source), a
// structured hostname, and a media section with plain-available services.
func shareCheckupData(t *testing.T) []byte {
	t.Helper()
	rep := checkup.CheckupReport{
		ReportKind: "checkup",
		System: sysinfo.SystemInfo{OS: sysinfo.OSInfo{
			Hostname: "prod-web-01", Name: "Ubuntu 24.04", Kernel: "6.8.0",
		}},
		NetworkInfo: checkup.NetworkInfoSection{
			SectionState: checkup.SectionState{Enabled: true, Status: "ok"},
			Result: &checkup.NetworkIdentityResult{
				PublicIPv4: &checkup.PublicIPIdentity{IP: "96.9.228.37", IPVersion: "v4", ASN: 54888},
				PublicIPv6: &checkup.PublicIPIdentity{IP: "2600:1f2:3:4::5", IPVersion: "v6"},
			},
		},
		Media: checkup.MediaSection{
			SectionState: checkup.SectionState{Enabled: true, Status: "ok"},
			Result: &checkup.MediaResult{
				Set:     "globe",
				Summary: checkup.MediaSummary{Available: 2, Blocked: 1},
				Items: []checkup.MediaServiceResult{
					{ID: "netflix", Title: "Netflix", Status: "available", Region: "na"},
					{ID: "gemini", Title: "Gemini", Status: "available", Region: "ai"},
					{ID: "openai", Title: "OpenAI", Status: "blocked", Region: "ai"},
				},
			},
		},
	}
	data, err := json.Marshal(rep)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// shareRunData builds a run report with a hostname and one workload.
func shareRunData(t *testing.T) []byte {
	t.Helper()
	doc := report.Document{
		System: sysinfo.SystemInfo{OS: sysinfo.OSInfo{
			Hostname: "bench-box-7", Name: "Debian 12", Kernel: "6.1.0",
		}},
		Results: report.ResultsSection{Workloads: []report.WorkloadEntry{{
			Name:     "CPU 1-thread",
			Category: "CPU",
			Result:   &report.ResultEntry{MedianMS: 12.3, ThroughputPerSec: 8123, ThroughputUnit: "ops/s"},
		}}},
	}
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestPrepareDetectsRunCheckupAndLegacySuite(t *testing.T) {
	tests := []struct {
		name string
		data func(t *testing.T) []byte
		kind history.Kind
	}{
		{name: "run", data: shareRunData, kind: history.KindRun},
		{name: "checkup", data: shareCheckupData, kind: history.KindCheckup},
		{name: "legacy suite", data: func(t *testing.T) []byte {
			return []byte(`{"schema_version":2,"report_kind":"suite","config":{},"media":{"enabled":true}}`)
		}, kind: history.KindCheckup},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prepared, err := Prepare(tt.data(t), Options{})
			if err != nil {
				t.Fatal(err)
			}
			if prepared.Kind != tt.kind {
				t.Fatalf("kind = %q, want %q", prepared.Kind, tt.kind)
			}
		})
	}
}

func TestPrepareRejectsUnknownJSON(t *testing.T) {
	_, err := Prepare([]byte(`{"hello":"world"}`), Options{})
	if err == nil {
		t.Fatal("Prepare accepted an unknown JSON document")
	}
	var notReport *NotReportError
	if !errors.As(err, &notReport) {
		t.Fatalf("error = %v, want NotReportError", err)
	}
}

func TestPrepareCheckupTextMasksPublicAddressesAndHostname(t *testing.T) {
	prepared, err := Prepare(shareCheckupData(t), Options{})
	if err != nil {
		t.Fatal(err)
	}
	payload := string(prepared.Payload)
	if strings.Contains(payload, "96.9.228.37") || strings.Contains(payload, "2600:1f2:3:4::5") {
		t.Fatalf("public address leaked into text payload:\n%s", payload)
	}
	if !strings.Contains(payload, "203.0.113.1") || !strings.Contains(payload, "2001:db8::1") {
		t.Fatalf("documentation placeholders missing from text payload:\n%s", payload)
	}
	if strings.Contains(payload, "prod-web-01") {
		t.Fatalf("hostname leaked into text payload:\n%s", payload)
	}
	if prepared.Inventory.IPv4Count != 1 || prepared.Inventory.IPv6Count != 1 {
		t.Fatalf("inventory counts = ipv4×%d ipv6×%d, want 1/1", prepared.Inventory.IPv4Count, prepared.Inventory.IPv6Count)
	}
	if prepared.Inventory.HostnameMasked != 1 {
		t.Fatalf("HostnameMasked = %d, want 1", prepared.Inventory.HostnameMasked)
	}
	if prepared.Inventory.Sites < 2 {
		t.Fatalf("Sites = %d, want every occurrence counted", prepared.Inventory.Sites)
	}
}

// TestPrepareFooterMatchesInventory pins the honesty rule: the footer only
// ever states what redaction actually did.
func TestPrepareFooterMatchesInventory(t *testing.T) {
	prepared, err := Prepare(shareCheckupData(t), Options{})
	if err != nil {
		t.Fatal(err)
	}
	summary := prepared.Inventory.Summary()
	if !strings.Contains(string(prepared.Payload), summary) {
		t.Fatalf("payload footer lacks inventory summary %q:\n%s", summary, prepared.Payload)
	}
	if !strings.Contains(summary, "ipv4×1") || !strings.Contains(summary, "hostname×1") {
		t.Fatalf("summary = %q, want address and hostname counts", summary)
	}

	// A report with nothing left to mask states that instead of claiming work.
	quiet := `{"schema_version":2,"report_kind":"suite","config":{},"media":{"enabled":true}}`
	prepared, err = Prepare([]byte(quiet), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got := prepared.Inventory.Summary(); got != i18n.T("cli.share.redactedNothing") {
		t.Fatalf("quiet summary = %q", got)
	}
}

func TestPrepareModeNoneKeepsPlaintext(t *testing.T) {
	prepared, err := Prepare(shareCheckupData(t), Options{Redact: redact.ModeNone})
	if err != nil {
		t.Fatal(err)
	}
	payload := string(prepared.Payload)
	if !strings.Contains(payload, "96.9.228.37") {
		t.Fatalf("ModeNone must keep plaintext addresses:\n%s", payload)
	}
	if got := prepared.Inventory.Summary(); got != i18n.T("cli.share.unredacted") {
		t.Fatalf("none-mode summary = %q", got)
	}

	// The markdown projection never prints the hostname; json does.
	prepared, err = Prepare(shareCheckupData(t), Options{Redact: redact.ModeNone, Format: FormatJSON})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(prepared.Payload), "prod-web-01") {
		t.Fatalf("ModeNone must keep the hostname in json:\n%s", prepared.Payload)
	}
}

func TestPrepareJSONFormatIsRedactedMarshalWithoutFooter(t *testing.T) {
	prepared, err := Prepare(shareCheckupData(t), Options{Format: FormatJSON})
	if err != nil {
		t.Fatal(err)
	}
	payload := string(prepared.Payload)
	if strings.Contains(payload, "96.9.228.37") || strings.Contains(payload, "prod-web-01") {
		t.Fatalf("json payload must be redacted:\n%s", payload)
	}
	if !strings.Contains(payload, `"hostname.redacted"`) {
		t.Fatalf("json payload must carry the masked hostname:\n%s", payload)
	}
	if strings.Contains(payload, "generated by") {
		t.Fatalf("json projection must not append the share footer:\n%s", payload)
	}
	var roundTrip map[string]any
	if err := json.Unmarshal(prepared.Payload, &roundTrip); err != nil {
		t.Fatalf("json payload is not valid JSON: %v", err)
	}
}

func TestPrepareRunTextCarriesWorkloadAndFooter(t *testing.T) {
	prepared, err := Prepare(shareRunData(t), Options{})
	if err != nil {
		t.Fatal(err)
	}
	payload := string(prepared.Payload)
	for _, want := range []string{"CPU 1-thread", "8123 ops/s", "generated by"} {
		if !strings.Contains(payload, want) {
			t.Fatalf("run payload missing %q:\n%s", want, payload)
		}
	}
	if prepared.Inventory.HostnameMasked != 1 {
		t.Fatalf("HostnameMasked = %d, want 1 for run reports too", prepared.Inventory.HostnameMasked)
	}
}

// TestPrepareIdempotentOnRedactedInput feeds a redacted json payload back
// through Prepare: documentation-range placeholders are not re-masked, so the
// payload survives unchanged.
func TestPrepareIdempotentOnRedactedInput(t *testing.T) {
	first, err := Prepare(shareCheckupData(t), Options{Format: FormatJSON})
	if err != nil {
		t.Fatal(err)
	}
	second, err := Prepare(first.Payload, Options{Format: FormatJSON})
	if err != nil {
		t.Fatal(err)
	}
	if string(second.Payload) != string(first.Payload) {
		t.Fatalf("second pass deformed the payload:\nfirst:  %s\nsecond: %s", first.Payload, second.Payload)
	}
	if second.Inventory.IPv4Count != 0 || second.Inventory.IPv6Count != 0 {
		t.Fatalf("second pass re-masked placeholders: %+v", second.Inventory)
	}
}

func TestPrepareMediaFullSwitchesToFullList(t *testing.T) {
	folded, err := Prepare(shareCheckupData(t), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(folded.Payload), "Netflix") {
		t.Fatalf("folded media must not list plain-available services:\n%s", folded.Payload)
	}
	full, err := Prepare(shareCheckupData(t), Options{MediaFull: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Netflix", "Gemini", "OpenAI"} {
		if !strings.Contains(string(full.Payload), want) {
			t.Fatalf("full media list missing %q:\n%s", want, full.Payload)
		}
	}
}

func TestPrepareRejectsOversizedPayload(t *testing.T) {
	doc := report.Document{
		Results: report.ResultsSection{Workloads: []report.WorkloadEntry{{
			Name:   "filler",
			Result: &report.ResultEntry{Detail: strings.Repeat("x", 600*1024)},
		}}},
	}
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	_, err = Prepare(data, Options{Format: FormatJSON})
	var tooLarge *TooLargeError
	if !errors.As(err, &tooLarge) {
		t.Fatalf("error = %v, want TooLargeError", err)
	}
	if tooLarge.Limit != maxPayloadBytes || tooLarge.Size <= maxPayloadBytes {
		t.Fatalf("TooLargeError fields = %+v", tooLarge)
	}
}

func TestParseFormat(t *testing.T) {
	for _, tt := range []struct {
		value string
		want  Format
	}{
		{value: "", want: FormatText},
		{value: "text", want: FormatText},
		{value: " JSON ", want: FormatJSON},
	} {
		got, err := ParseFormat(tt.value)
		if err != nil || got != tt.want {
			t.Fatalf("ParseFormat(%q) = %q, %v; want %q", tt.value, got, err, tt.want)
		}
	}
	if _, err := ParseFormat("yaml"); err == nil || !strings.Contains(err.Error(), "json") {
		t.Fatalf("ParseFormat(yaml) = %v, want error listing options", err)
	}
}
