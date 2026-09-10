package checkup

import (
	"encoding/json"
	"strings"
	"testing"

	vmbench "github.com/cloudapp3/vmbench"
	"github.com/cloudapp3/vmbench/bench/netio"
	"github.com/cloudapp3/vmbench/redact"
	gbreport "github.com/cloudapp3/vmbench/report"
)

// redactFixture builds a checkup report carrying every known public-IP shape:
// structured identity fields, free-text evidence, reverse DNSBL labels,
// prefixes, and an embedded hardware run document.
func redactFixture() CheckupReport {
	report := CheckupReport{}
	report.NetworkInfo.Result = &NetworkIdentityResult{
		PublicIPv4: &PublicIPIdentity{IP: "96.9.228.37", IPVersion: "v4", ASN: 54888},
		PublicIPv6: &PublicIPIdentity{IP: "2600:1f2:3:4::5", IPVersion: "v6"},
		LocalGlobalAddresses: []LocalGlobalAddress{
			{Interface: "eth0", Address: "10.0.0.5", IPVersion: "v4", Private: true},
		},
		NAT: []NATHeuristic{{
			IPVersion: "v4",
			Status:    "translated",
			PublicIP:  "96.9.228.37",
			LocalIP:   "10.0.0.5",
		}},
		CIDRNeighbors: &netio.CIDRNeighborsEvidence{
			IPv4:            "96.9.228.37",
			SubnetPrefix:    "96.9.228.0/24",
			AnnouncedPrefix: "96.9.228.0/23",
		},
		IPBGP: &netio.IPBGPEvidence{
			IP:       "96.9.228.37",
			Prefixes: []string{"96.9.228.0/23"},
			Range:    "96.9.228.0 - 96.9.229.255",
		},
	}
	report.IPQuality.Result = &IPQualityResult{
		BasicInfo: &IPBasicInfo{IP: "96.9.228.37", Country: "US"},
		SecurityCheck: &netio.SecurityCheckResult{
			Status: "ok",
			Raw:    "IP: 96.9.228.37 native, 2600:1f2:3:4::5",
		},
	}
	report.Hardware.Report = &vmbench.Report{
		Results: gbreport.ResultsSection{Workloads: []gbreport.WorkloadEntry{{
			Name:        "net",
			Category:    "network",
			Description: "detail fixture",
			Result:      &gbreport.ResultEntry{MedianMS: 3.5, Detail: "egress via 96.9.228.37"},
		}}},
	}
	return report
}

func TestRedactReportMasksEveryOccurrence(t *testing.T) {
	report := redactFixture()
	redactReport(&report, redact.Default)
	raw, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)

	if strings.Contains(text, "96.9.228") || strings.Contains(text, "2600:1f2:3:4::5") {
		t.Fatalf("public address leaked:\n%s", text)
	}
	for _, want := range []string{
		"203.0.113.1",                         // v4 placeholder
		"2001:db8::1",                         // v6 placeholder
		"10.0.0.5",                            // private local address kept
		`"subnet_prefix":"203.0.113.0/24"`,    // subnet prefix masked, length kept
		`"announced_prefix":"203.0.113.0/24"`, // /23 collapsed to /24
		`"prefixes":["203.0.113.0/24"]`,       // BGP prefix list masked
		`"range":"203.0.113.0/24"`,            // registration range → prefix form
		`"detail":"egress via 203.0.113.1"`,   // embedded run document Detail
	} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in\n%s", want, text)
		}
	}
}

func TestRedactReportMasksReverseDNSBLEvidence(t *testing.T) {
	report := redactFixture()
	report.IPQuality.Result.RiskSummary = &IPRiskSummary{
		DNSBLMessage: "lookup 37.228.9.96.zen.spamhaus.org: i/o timeout",
	}
	redactReport(&report, redact.Default)
	raw, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if strings.Contains(text, "37.228.9.96") {
		t.Fatalf("reverse DNSBL label leaked:\n%s", text)
	}
	if !strings.Contains(text, "1.113.0.203.zen.spamhaus.org") {
		t.Errorf("reverse label not masked in\n%s", text)
	}
}

func TestRedactReportModeNoneKeepsPlaintext(t *testing.T) {
	report := redactFixture()
	redactReport(&report, redact.ModeNone)
	raw, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "96.9.228.37") {
		t.Errorf("ModeNone should keep plaintext addresses:\n%s", raw)
	}
}

func TestRedactReportKeepsMetricsThroughRoundTrip(t *testing.T) {
	report := redactFixture()
	redactReport(&report, redact.Default)
	entry := report.Hardware.Report.Results.Workloads[0].Result
	if entry.MedianMS != 3.5 {
		t.Errorf("MedianMS = %v, want 3.5", entry.MedianMS)
	}
	if report.NetworkInfo.Result.PublicIPv4.ASN != 54888 {
		t.Errorf("ASN = %v, want kept", report.NetworkInfo.Result.PublicIPv4.ASN)
	}
	if report.NetworkInfo.Result.LocalGlobalAddresses[0].Address != "10.0.0.5" {
		t.Errorf("private local address = %q, want kept", report.NetworkInfo.Result.LocalGlobalAddresses[0].Address)
	}
}

func TestRedactDefaultsAndValidation(t *testing.T) {
	if got := PrepareOptions(Options{}).Redact; got != redact.Default {
		t.Errorf("PrepareOptions zero Redact = %q, want %q", got, redact.Default)
	}
	if err := ValidateOptions(Options{Redact: "bogus"}); err == nil || !strings.Contains(err.Error(), "redact") {
		t.Errorf("ValidateOptions should reject invalid redact mode, got %v", err)
	}
	if err := ValidateOptions(Options{Redact: redact.ModeNone}); err != nil {
		t.Errorf("ValidateOptions should accept none, got %v", err)
	}
}
