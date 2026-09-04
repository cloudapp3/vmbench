package netio

import (
	"testing"
	"time"

	bgptools "github.com/oneclickvirt/backtrace/bgptools"
)

func TestPopulateIPBGPFromReport(t *testing.T) {
	registered := time.Date(2022, 11, 16, 0, 0, 0, 0, time.UTC)
	report := &bgptools.IPBGPReport{
		IP:     "96.9.228.95",
		ASN:    "133752",
		Status: "available",
		RDAP: &bgptools.RDAPRecord{
			Name:             "365GROUP-GL-718",
			Country:          "HK",
			StartAddress:     "96.9.228.0",
			EndAddress:       "96.9.229.255",
			RegistrationDate: &registered,
			Entities:         []bgptools.RDAPEntity{{Handle: "GL-718", Roles: []string{"registrant"}}},
		},
		Prefixes: []string{"96.9.228.0/23"},
		RIR:      bgptools.RIRInfo{Name: "ARIN", Source: "rdap", Status: "available"},
		Relationships: &bgptools.ASNRelationshipReport{
			TargetASN: "133752",
			Status:    "available",
			Upstreams: []bgptools.ASNRelationship{
				{ASN: "AS174", Name: "Cogent", Kind: "upstream", Source: "ripestat", Status: "available"},
				{ASN: "AS9999", Name: "Ghost", Kind: "upstream", Source: "ripestat", Status: "rate_limited"},
			},
			Peers: []bgptools.ASNRelationship{
				{ASN: "6939", Name: "Hurricane Electric", Kind: "peer", Source: "peeringdb", Status: "available"},
			},
			IXPs: []bgptools.ASNRelationship{
				{ASN: "", Name: "HKIX", Kind: "ixp", Source: "peeringdb", Status: "available", IXPID: "ix-1"},
			},
		},
	}
	evidence := &IPBGPEvidence{IP: report.IP}
	populateIPBGPFromReport(evidence, report)

	if evidence.ASN != "AS133752" {
		t.Errorf("ASN = %q, want AS133752", evidence.ASN)
	}
	// The registrant handle (GL-718) is an organization identifier, not a
	// registry; RIR must come from the registry inference instead.
	if evidence.NetworkName != "365GROUP-GL-718" || evidence.RIR != "ARIN" {
		t.Errorf("ownership not carried: %+v", evidence)
	}
	if evidence.RegistrationDate != "2022-11-16" {
		t.Errorf("registration date = %q", evidence.RegistrationDate)
	}
	if evidence.Range != "96.9.228.0 - 96.9.229.255" {
		t.Errorf("range = %q", evidence.Range)
	}
	// rate_limited entries must be dropped; ok entries kept.
	if len(evidence.Relationships) != 3 {
		t.Fatalf("relationships = %d, want 3: %+v", len(evidence.Relationships), evidence.Relationships)
	}
	if evidence.Relationships[0].ASN != "AS174" || !evidence.Relationships[0].Tier1 {
		t.Errorf("tier1 upstream not flagged: %+v", evidence.Relationships[0])
	}
	if evidence.Tier1Upstreams != 1 {
		t.Errorf("tier1 upstreams = %d, want 1", evidence.Tier1Upstreams)
	}
	if evidence.Status != "ok" {
		t.Errorf("status = %q, want ok", evidence.Status)
	}
}

func TestPopulateIPBGPEmptyReport(t *testing.T) {
	evidence := &IPBGPEvidence{IP: "192.0.2.1"}
	populateIPBGPFromReport(evidence, &bgptools.IPBGPReport{IP: "192.0.2.1"})
	if evidence.Status != "error" || evidence.Message == "" {
		t.Fatalf("empty report should be structured error: %+v", evidence)
	}
}

func TestFormatASNumber(t *testing.T) {
	cases := map[string]string{
		"133752":   "AS133752",
		"as133752": "AS133752",
		"AS133752": "AS133752",
		"":         "",
		"  ":       "",
	}
	for in, want := range cases {
		if got := formatASNumber(in); got != want {
			t.Errorf("formatASNumber(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestProbeIPBGPWithoutAddress(t *testing.T) {
	evidence := ProbeIPBGP(nil, "  ")
	if evidence.Status != "error" || evidence.Message == "" {
		t.Fatalf("empty address should fail closed: %+v", evidence)
	}
}
