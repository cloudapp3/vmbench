package suite

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestSummarizeNetworkIdentityMarksMetadataFailurePartial(t *testing.T) {
	result := &NetworkIdentityResult{
		PublicIPv4: &PublicIPIdentity{IP: "198.51.100.20", IPVersion: "v4"},
		Providers: []NetworkIdentityProviderResult{
			{ID: "local_interfaces", Status: "ok"},
			{ID: "ipify_v4", Status: "ok"},
			{ID: "ipwhois_v4", Status: "error", Error: "offline"},
		},
	}
	status, message := summarizeNetworkIdentity(result, nil)
	if status != "partial" {
		t.Fatalf("status = %q, want partial", status)
	}
	if !strings.Contains(message, "2/3 providers ok") {
		t.Fatalf("message = %q, want provider counts", message)
	}
}

func TestSummarizeNetworkIdentityKeepsStructuredProbeError(t *testing.T) {
	result := &NetworkIdentityResult{Providers: []NetworkIdentityProviderResult{
		{ID: "local_interfaces", Status: "ok"},
		{ID: "ipify_v4", Status: "error", Error: "offline"},
		{ID: "ipwhois_v4", Status: "skipped"},
	}}
	status, message := summarizeNetworkIdentity(result, errors.New("no public IP"))
	if status != "partial" || !strings.Contains(message, "no public IP") {
		t.Fatalf("summarizeNetworkIdentity() = %q, %q; want partial with error", status, message)
	}
}

func TestSummarizeNetworkIdentityTreatsEmptyFailedProbeAsError(t *testing.T) {
	status, message := summarizeNetworkIdentity(&NetworkIdentityResult{}, errors.New("invalid probe"))
	if status != "error" || !strings.Contains(message, "invalid probe") {
		t.Fatalf("summarizeNetworkIdentity() = %q, %q; want error", status, message)
	}
}

func TestSummarizeReachabilityUsesPartialAndErrorStates(t *testing.T) {
	partialStatus, _ := summarizeReachability([]ReachabilityResult{
		{Status: "reachable"},
		{Status: "unreachable"},
	})
	if partialStatus != "partial" {
		t.Fatalf("partial status = %q, want partial", partialStatus)
	}
	errorStatus, _ := summarizeReachability([]ReachabilityResult{{Status: "http_error"}})
	if errorStatus != "error" {
		t.Fatalf("error status = %q, want error", errorStatus)
	}
}

func TestWriteConsoleIncludesNetworkEvidence(t *testing.T) {
	report := SuiteReport{
		Status:  "failed",
		Message: "partial evidence",
		NetworkInfo: NetworkInfoSection{
			SectionState: SectionState{Enabled: true, Status: "partial", Message: "2/3 providers ok"},
			Result: &NetworkIdentityResult{
				PublicIPv4: &PublicIPIdentity{IP: "198.51.100.20", IPVersion: "v4", ASN: 64500, Org: "Example Net", CountryCode: "US"},
				NAT:        []NATHeuristic{{IPVersion: "v4", Status: "translated", PublicIP: "198.51.100.20", LocalIP: "10.0.0.2", Reason: "mismatch"}},
				Providers:  []NetworkIdentityProviderResult{{ID: "ipwhois_v4", Kind: "metadata", IPVersion: "v4", Status: "error", Error: "offline"}},
			},
		},
		Reachability: ReachabilitySection{
			SectionState: SectionState{Enabled: true, Status: "partial", Message: "1/2 targets reachable"},
			Results:      []ReachabilityResult{{ID: "github", Category: "website", Protocol: "https", Endpoint: "https://github.com/", Status: "reachable", HTTPStatus: 200, LatencyMs: 12.5}},
		},
	}
	var output bytes.Buffer
	if err := WriteConsole(&output, report); err != nil {
		t.Fatalf("WriteConsole() error = %v", err)
	}
	got := output.String()
	for _, want := range []string{"[Network Info]", "AS64500", "translated", "ipwhois_v4", "[Reachability]", "https://github.com/", "12.5 ms"} {
		if !strings.Contains(got, want) {
			t.Fatalf("console output missing %q:\n%s", want, got)
		}
	}
}
