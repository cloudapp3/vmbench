package netio

import (
	"testing"

	"github.com/oneclickvirt/gostun/stuncheck"
)

func TestNATEvidenceFromSummaryAllOK(t *testing.T) {
	summary := &stuncheck.NATSummary{
		Status: stuncheck.CapabilityAvailable,
		Results: []stuncheck.NATReport{{
			Server: "stun-a:3478", Status: stuncheck.CapabilityAvailable,
			NATType: "Full Cone", MappingBehavior: "Endpoint-Independent",
			FilteringBehavior: "Endpoint-Independent", PortPreservation: stuncheck.CapabilityAvailable,
			Hairpin: stuncheck.CapabilityAvailable,
		}, {
			Server: "stun-b:3478", Status: stuncheck.CapabilityAvailable,
			NATType: "Full Cone",
		}},
		Successful: 2,
	}
	evidence := natEvidenceFromSummary(summary)
	if evidence.Status != "ok" || evidence.NATType != "Full Cone" {
		t.Fatalf("status/type = %s/%s, want ok/Full Cone", evidence.Status, evidence.NATType)
	}
	if evidence.MappingBehavior != "Endpoint-Independent" || evidence.Hairpin != "available" {
		t.Fatalf("behaviors not carried: %+v", evidence)
	}
	if len(evidence.Results) != 2 {
		t.Fatalf("server results = %d, want 2", len(evidence.Results))
	}
}

func TestNATEvidenceFromSummaryPartial(t *testing.T) {
	summary := &stuncheck.NATSummary{
		Status:  stuncheck.CapabilityUnavailable,
		Partial: true,
		Results: []stuncheck.NATReport{
			{Server: "stun-a:3478", Status: stuncheck.CapabilityAvailable, NATType: "Restricted Cone"},
			{Server: "stun-b:3478", Status: stuncheck.CapabilityTimeout, Error: "i/o timeout"},
		},
		Successful: 1,
		Failed:     1,
	}
	evidence := natEvidenceFromSummary(summary)
	if evidence.Status != "partial" || evidence.NATType != "Restricted Cone" || !evidence.Partial {
		t.Fatalf("unexpected evidence: %+v", evidence)
	}
}

func TestNATEvidenceFromSummaryAllFailed(t *testing.T) {
	summary := &stuncheck.NATSummary{
		Status: stuncheck.CapabilityError,
		Results: []stuncheck.NATReport{
			{Server: "stun-a:3478", Status: stuncheck.CapabilityTimeout},
		},
		Failed: 1,
		Error:  "all servers failed",
	}
	evidence := natEvidenceFromSummary(summary)
	if evidence.Status != "error" || evidence.Message != "all servers failed" {
		t.Fatalf("unexpected evidence: %+v", evidence)
	}
	if evidence.NATType != "" {
		t.Errorf("failed probe should not claim a NAT type, got %q", evidence.NATType)
	}
}

func TestNATEvidenceFromSummaryNoResponses(t *testing.T) {
	evidence := natEvidenceFromSummary(&stuncheck.NATSummary{})
	if evidence.Status != "unsupported" || evidence.NATType != "Inconclusive" {
		t.Fatalf("unexpected evidence: %+v", evidence)
	}
}

func TestNATEvidenceFromNilSummary(t *testing.T) {
	if evidence := natEvidenceFromSummary(nil); evidence.Status != "error" {
		t.Fatalf("nil summary status = %s, want error", evidence.Status)
	}
}
