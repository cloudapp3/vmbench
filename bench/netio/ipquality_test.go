package netio

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
)

func TestProbeIPQualityFailsClosedWhenMetadataUnavailable(t *testing.T) {
	deps := validIPQualityDependencies()
	deps.queryInfo = func(context.Context) (*IPBasicInfo, riskFlags, error) {
		return nil, riskFlags{}, errors.New("metadata offline")
	}

	result, err := probeIPQuality(context.Background(), deps)
	if err == nil || !strings.Contains(err.Error(), "metadata unavailable") {
		t.Fatalf("probeIPQuality() error = %v, want metadata unavailable", err)
	}
	if result == nil {
		t.Fatal("probeIPQuality() result = nil, want partial structured result")
	}
	if result.Score != nil {
		t.Fatalf("probeIPQuality() Score = %+v, want nil on metadata failure", result.Score)
	}
	if result.BasicInfo == nil || !strings.Contains(result.BasicInfo.Error, "metadata offline") {
		t.Fatalf("probeIPQuality() BasicInfo = %+v, want structured metadata error", result.BasicInfo)
	}
}

func TestProbeIPQualityFailsClosedWhenPublicIPUnavailable(t *testing.T) {
	deps := validIPQualityDependencies()
	deps.queryInfo = func(context.Context) (*IPBasicInfo, riskFlags, error) {
		return &IPBasicInfo{Source: "ip-api.com"}, riskFlags{}, nil
	}
	deps.publicIP = func(context.Context) (string, error) {
		return "", errors.New("public IP offline")
	}

	result, err := probeIPQuality(context.Background(), deps)
	if err == nil || !strings.Contains(err.Error(), "public IPv4 unavailable") {
		t.Fatalf("probeIPQuality() error = %v, want public IPv4 unavailable", err)
	}
	if result.Score != nil {
		t.Fatalf("probeIPQuality() Score = %+v, want nil on public IP failure", result.Score)
	}
}

func TestProbeIPQualityFailsClosedWhenDNSBLUnavailable(t *testing.T) {
	deps := validIPQualityDependencies()
	deps.checkDNSBL = func(context.Context, string) dnsblCheckResult {
		return dnsblCheckResult{Errors: []string{"zen.spamhaus.org: DNS timeout"}}
	}

	result, err := probeIPQuality(context.Background(), deps)
	if err == nil || !strings.Contains(err.Error(), "DNSBL unavailable") {
		t.Fatalf("probeIPQuality() error = %v, want DNSBL unavailable", err)
	}
	if result.Score != nil {
		t.Fatalf("probeIPQuality() Score = %+v, want nil on DNSBL failure", result.Score)
	}
	if result.RiskSummary == nil || result.RiskSummary.DNSBLSupported {
		t.Fatalf("probeIPQuality() RiskSummary = %+v, want unsupported DNSBL", result.RiskSummary)
	}
}

func TestProbeIPQualityFailsClosedWhenDNSBLIsPartial(t *testing.T) {
	deps := validIPQualityDependencies()
	deps.checkDNSBL = func(context.Context, string) dnsblCheckResult {
		return dnsblCheckResult{Checked: len(dnsblZones) - 1, Errors: []string{"one zone timed out"}}
	}

	result, err := probeIPQuality(context.Background(), deps)
	if err == nil || !strings.Contains(err.Error(), "DNSBL unavailable") {
		t.Fatalf("probeIPQuality() error = %v, want partial DNSBL failure", err)
	}
	if result.Score != nil {
		t.Fatalf("probeIPQuality() Score = %+v, want nil on partial DNSBL failure", result.Score)
	}
}

func TestProbeIPQualityScoresOnlyConclusiveInputs(t *testing.T) {
	deps := validIPQualityDependencies()
	result, err := probeIPQuality(context.Background(), deps)
	if err != nil {
		t.Fatalf("probeIPQuality() error = %v", err)
	}
	if result.Score == nil || result.Score.Total != 100 {
		t.Fatalf("probeIPQuality() Score = %+v, want 100/100", result.Score)
	}
}

func TestCheckDNSBLDistinguishesNotListedFromLookupFailure(t *testing.T) {
	notFound := &net.DNSError{Err: "no such host", IsNotFound: true}
	lookup := func(_ context.Context, host string) ([]string, error) {
		switch {
		case strings.HasSuffix(host, dnsblZones[0]):
			return nil, notFound
		case strings.HasSuffix(host, dnsblZones[1]):
			return []string{"127.0.0.2"}, nil
		default:
			return nil, errors.New("resolver offline")
		}
	}

	result := checkDNSBLWithLookup(context.Background(), "192.0.2.10", lookup)
	if result.Checked != 2 {
		t.Fatalf("Checked = %d, want 2", result.Checked)
	}
	if len(result.Listed) != 1 || result.Listed[0] != dnsblZones[1] {
		t.Fatalf("Listed = %v, want [%s]", result.Listed, dnsblZones[1])
	}
	if len(result.Errors) != 1 || !strings.Contains(result.Errors[0], "resolver offline") {
		t.Fatalf("Errors = %v, want resolver failure", result.Errors)
	}
}

func validIPQualityDependencies() ipQualityDependencies {
	return ipQualityDependencies{
		queryInfo: func(context.Context) (*IPBasicInfo, riskFlags, error) {
			return &IPBasicInfo{Source: "ip-api.com", IP: "192.0.2.10"}, riskFlags{}, nil
		},
		publicIP: func(context.Context) (string, error) {
			return "192.0.2.10", nil
		},
		checkDNSBL: func(context.Context, string) dnsblCheckResult {
			return dnsblCheckResult{Checked: len(dnsblZones)}
		},
		mailPorts: func(context.Context, []int) []PortProbe {
			return []PortProbe{{Port: 25, Status: "open"}}
		},
	}
}
