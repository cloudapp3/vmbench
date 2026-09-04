package netio

import (
	"context"
	"fmt"
	"strings"
	"time"

	bgptools "github.com/oneclickvirt/backtrace/bgptools"
)

// BGPRelationship is one normalized upstream/peer/IXP entry.
type BGPRelationship struct {
	ASN    string `json:"asn,omitempty"`
	Name   string `json:"name,omitempty"`
	Kind   string `json:"kind,omitempty"`
	Source string `json:"source,omitempty"`
	Tier1  bool   `json:"tier1,omitempty"`
}

// IPBGPEvidence stores the public BGP/RDAP view of one address: announcing
// ASN, registered prefix range, RIR/registration metadata, and the upstream /
// peer / IXP relationships from RIPEstat and PeeringDB (via the bgptools
// package of backtrace, Apache-2.0). It is supplementary evidence and never
// gates the network identity section.
type IPBGPEvidence struct {
	Status           string            `json:"status"` // ok | partial | error
	IP               string            `json:"ip,omitempty"`
	ASN              string            `json:"asn,omitempty"`
	NetworkName      string            `json:"network_name,omitempty"`
	Prefixes         []string          `json:"prefixes,omitempty"`
	Range            string            `json:"range,omitempty"`
	RIR              string            `json:"rir,omitempty"`
	Country          string            `json:"country,omitempty"`
	RegistrationDate string            `json:"registration_date,omitempty"`
	GeofeedURLs      []string          `json:"geofeed_urls,omitempty"`
	Relationships    []BGPRelationship `json:"relationships,omitempty"`
	Tier1Upstreams   int               `json:"tier1_upstreams,omitempty"`
	Message          string            `json:"message,omitempty"`
}

// ipBGPTimeout bounds the combined RDAP + relationship queries.
const ipBGPTimeout = 30 * time.Second

// tier1ASNs is the publicly documented set of Tier 1 transit-free networks.
// It is factual industry data used only as a display hint on relationships.
var tier1ASNs = map[string]struct{}{
	"AS174": {}, "AS701": {}, "AS702": {}, "AS7018": {}, "AS1299": {},
	"AS2914": {}, "AS3257": {}, "AS3320": {}, "AS3356": {}, "AS5511": {},
	"AS6453": {}, "AS6461": {}, "AS6762": {}, "AS6830": {}, "AS8220": {},
}

// ProbeIPBGP collects the RDAP/BGP view for one public address. Failures are
// structured evidence, never a probe error.
func ProbeIPBGP(ctx context.Context, ip string) *IPBGPEvidence {
	evidence := &IPBGPEvidence{IP: strings.TrimSpace(ip)}
	if evidence.IP == "" {
		evidence.Status = "error"
		evidence.Message = "no public address to query"
		return evidence
	}
	if ctx == nil {
		ctx = context.Background()
	}
	queryCtx, cancel := context.WithTimeout(ctx, ipBGPTimeout)
	defer cancel()
	report, err := bgptools.QueryIPBGP(queryCtx, evidence.IP, bgptools.IPBGPReportConfig{
		Timeout:             ipBGPTimeout,
		FetchGeofeed:        true,
		GeofeedTimeout:      5 * time.Second,
		MaxGeofeedBytes:     64 << 10,
		EnableWHOISFallback: true,
		WHOISTimeout:        5 * time.Second,
		MaxWHOISBytes:       64 << 10,
		WHOISBootstrapURL:   "",
		ResolveASN:          bgptools.ResolveOriginASN,
		Relationships:       bgptools.RelationshipConfig{Timeout: ipBGPTimeout},
	})
	if report != nil {
		populateIPBGPFromReport(evidence, report)
	}
	if err != nil {
		if evidence.Status == "" {
			evidence.Status = "error"
		}
		evidence.Message = strings.TrimSpace(strings.Join([]string{err.Error(), evidence.Message}, "; "))
		return evidence
	}
	if evidence.Status == "" {
		evidence.Status = "ok"
	}
	return evidence
}

func populateIPBGPFromReport(evidence *IPBGPEvidence, report *bgptools.IPBGPReport) {
	evidence.ASN = formatASNumber(report.ASN)
	if report.RDAP != nil {
		evidence.NetworkName = report.RDAP.Name
		evidence.Country = report.RDAP.Country
		if report.RDAP.RegistrationDate != nil {
			evidence.RegistrationDate = report.RDAP.RegistrationDate.Format(time.DateOnly)
		}
		if report.RDAP.StartAddress != "" || report.RDAP.EndAddress != "" {
			evidence.Range = report.RDAP.StartAddress + " - " + report.RDAP.EndAddress
		}
		for _, entity := range report.RDAP.Entities {
			for _, role := range entity.Roles {
				if strings.EqualFold(role, "registrant") && entity.Handle != "" {
					evidence.RIR = firstNonEmpty(entity.Handle, evidence.RIR)
				}
			}
		}
	}
	if rir := strings.TrimSpace(report.RIR.Name); rir != "" && evidence.RIR == "" {
		evidence.RIR = rir
	}
	evidence.Prefixes = append([]string(nil), report.Prefixes...)
	for _, geofeed := range report.Geofeeds {
		if strings.TrimSpace(geofeed.URL) != "" {
			evidence.GeofeedURLs = append(evidence.GeofeedURLs, geofeed.URL)
		}
	}
	if report.RegistrationDate != nil && evidence.RegistrationDate == "" {
		evidence.RegistrationDate = report.RegistrationDate.Format(time.DateOnly)
	}
	if rel := report.Relationships; rel != nil {
		evidence.Status = "ok"
		for _, group := range []struct {
			entries []bgptools.ASNRelationship
		}{{rel.Upstreams}, {rel.Peers}, {rel.IXPs}} {
			for _, entry := range group.entries {
				if !relationshipAvailable(entry) {
					continue
				}
				relationship := BGPRelationship{
					ASN:    formatASNumber(entry.ASN),
					Name:   entry.Name,
					Kind:   string(entry.Kind),
					Source: entry.Source,
				}
				if _, ok := tier1ASNs[strings.ToUpper(strings.TrimSpace(entry.ASN))]; ok {
					relationship.Tier1 = true
					if strings.EqualFold(relationship.Kind, "upstream") {
						evidence.Tier1Upstreams++
					}
				}
				evidence.Relationships = append(evidence.Relationships, relationship)
			}
		}
		if rel.RateLimited || rel.Timeout {
			evidence.Status = "partial"
			if rel.RateLimited {
				evidence.Message = "relationship providers rate limited"
			} else {
				evidence.Message = "relationship providers timed out"
			}
		}
	}
	if evidence.ASN == "" && len(evidence.Prefixes) == 0 {
		evidence.Status = "error"
		evidence.Message = fmt.Sprintf("no RDAP/BGP data for %s", evidence.IP)
	}
}

// relationshipAvailable accepts the bgptools success statuses: entries may be
// reported as "available" or legacy "ok".
func relationshipAvailable(entry bgptools.ASNRelationship) bool {
	status := strings.ToLower(strings.TrimSpace(string(entry.Status)))
	return status == "available" || status == "ok"
}

// formatASNumber normalizes "133752" and "AS133752" spellings to "AS133752".
func formatASNumber(asn string) string {
	trimmed := strings.ToUpper(strings.TrimSpace(asn))
	if trimmed == "" {
		return ""
	}
	if strings.HasPrefix(trimmed, "AS") {
		return trimmed
	}
	return "AS" + trimmed
}
