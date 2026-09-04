package netio

import (
	"context"
	"strings"
	"testing"
)

func TestIPQualityOptionsHasSource(t *testing.T) {
	if !(IPQualityOptions{}).HasSource(IPSourceBuiltin) {
		t.Error("empty options should default to builtin")
	}
	if (IPQualityOptions{}).HasSource(IPSourceSecurityCheck) {
		t.Error("empty options should not enable securitycheck")
	}
	opts := IPQualityOptions{Sources: []string{"builtin", "securitycheck"}}
	if !opts.HasSource(IPSourceSecurityCheck) || !opts.HasSource(IPSourceBuiltin) {
		t.Error("explicit sources should be honored")
	}
	mixed := IPQualityOptions{Sources: []string{" SecurityCheck "}}
	if !mixed.HasSource(IPSourceSecurityCheck) {
		t.Error("source matching should be case/whitespace tolerant")
	}
}

func TestValidateIPSources(t *testing.T) {
	if err := ValidateIPSources([]string{"builtin", "securitycheck"}); err != nil {
		t.Errorf("valid sources rejected: %v", err)
	}
	if err := ValidateIPSources([]string{"bogus"}); err == nil {
		t.Error("unknown source should be rejected")
	}
}

func TestQueryIPAPIISParseAndFailures(t *testing.T) {
	original := ipapiisBaseURL
	defer func() { ipapiisBaseURL = original }()

	ipapiisBaseURL = "unsupported://example.invalid"
	info := queryIPAPIIS(context.Background(), "192.0.2.10")
	if info.Supported {
		t.Fatal("query should be unsupported on transport failure")
	}
	if info.Message == "" {
		t.Error("failure message should be recorded")
	}
}

func TestIPAPIISCrossCheck(t *testing.T) {
	agree := &IPAPIISInfo{Supported: true, Company: "Example Org LLC", ASN: "AS64512 Example Org"}
	basic := &IPBasicInfo{Org: "EXAMPLE ORG", ASN: 64512}
	if note := agree.CrossCheck(basic); strings.Contains(note, "mismatch") {
		t.Errorf("expected agreement, got %q", note)
	}

	disagree := &IPAPIISInfo{Supported: true, Company: "Other Company", ASN: "AS64513 Other"}
	note := disagree.CrossCheck(basic)
	if !strings.Contains(note, "company mismatch") || !strings.Contains(note, "asn mismatch") {
		t.Errorf("expected mismatch evidence, got %q", note)
	}
}

func TestProbeIPQualityRecordsSupplementarySources(t *testing.T) {
	deps := validIPQualityDependencies()
	deps.queryIPAPIIS = func(context.Context, string) *IPAPIISInfo {
		return &IPAPIISInfo{Supported: true, Company: "Example Org LLC", ASN: "AS64512 Example Org"}
	}
	deps.runSecurityCheck = func(context.Context) *SecurityCheckResult {
		return &SecurityCheckResult{Source: IPSourceSecurityCheck, Status: "unavailable", Message: "not installed"}
	}

	result, err := probeIPQuality(context.Background(), deps, IPQualityOptions{Sources: []string{"builtin", "securitycheck"}})
	if err != nil {
		t.Fatalf("probeIPQuality() error = %v", err)
	}
	if result.IPAPIIS == nil || !result.IPAPIIS.Supported {
		t.Fatalf("ipapi.is evidence missing: %+v", result.IPAPIIS)
	}
	if result.SecurityCheck == nil || result.SecurityCheck.Status != "unavailable" {
		t.Fatalf("securitycheck status missing or wrong: %+v", result.SecurityCheck)
	}
	found := map[string]string{}
	for _, source := range result.Sources {
		found[source.Source] = source.Status
	}
	if found[IPSourceBuiltin] != "ok" || found["ipapi.is"] != "ok" || found[IPSourceSecurityCheck] != "unavailable" {
		t.Fatalf("unexpected source statuses: %+v", result.Sources)
	}
	// Supplementary sources must not alter the clean-IP score.
	if result.Score == nil || result.Score.Total != 100 {
		t.Fatalf("score changed by supplementary sources: %+v", result.Score)
	}
}

func TestProbeIPQualitySkipsSecurityCheckUnlessRequested(t *testing.T) {
	deps := validIPQualityDependencies()
	called := false
	deps.runSecurityCheck = func(context.Context) *SecurityCheckResult {
		called = true
		return &SecurityCheckResult{Status: "ok"}
	}
	if _, err := probeIPQuality(context.Background(), deps, IPQualityOptions{}); err != nil {
		t.Fatalf("probeIPQuality() error = %v", err)
	}
	if called {
		t.Error("securitycheck must not run unless explicitly enabled")
	}
}

func TestRunSecurityCheckMissingBinary(t *testing.T) {
	original := scBinaryBaseCandidates
	defer func() { scBinaryBaseCandidates = original }()
	scBinaryBaseCandidates = "definitely-not-installed-vmbench-sc"

	result := runSecurityCheck(context.Background())
	if result.Status != "unavailable" {
		t.Fatalf("status = %q, want unavailable", result.Status)
	}
	if result.Message == "" {
		t.Error("unavailable result should explain how to install the binary")
	}
}

func TestTrimSCRawStripsANSIAndTruncates(t *testing.T) {
	colored := "\x1b[32mFraud Score: 0\x1b[0m\n"
	cleaned := trimSCRaw([]byte(colored))
	if strings.Contains(cleaned, "\x1b") || strings.TrimSpace(cleaned) != "Fraud Score: 0" {
		t.Errorf("ANSI stripping failed: %q", cleaned)
	}
	long := strings.Repeat("x", scRawOutputLimit+100)
	truncated := trimSCRaw([]byte(long))
	if len(truncated) > scRawOutputLimit+64 {
		t.Errorf("raw output not truncated: %d", len(truncated))
	}
}

func TestSCInterestingFieldExtraction(t *testing.T) {
	result := &SecurityCheckResult{Status: "ok", Raw: "Fraud Score: 12\nIP Type: Data Center\nRandom Line: nope"}
	for _, line := range strings.Split(result.Raw, "\n") {
		match := scFieldRe.FindStringSubmatch(line)
		if len(match) != 3 {
			continue
		}
		if scInterestingField(match[1]) {
			result.Fields = append(result.Fields, SCField{Name: match[1], Value: match[2]})
		}
	}
	if len(result.Fields) != 2 {
		t.Fatalf("extracted %d fields, want 2: %+v", len(result.Fields), result.Fields)
	}
}
