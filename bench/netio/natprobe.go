package netio

import (
	"context"
	"strings"
	"time"

	"github.com/oneclickvirt/gostun/model"
	"github.com/oneclickvirt/gostun/stuncheck"
)

// NATServerResult records one STUN server view of the NAT.
type NATServerResult struct {
	Server  string `json:"server"`
	Status  string `json:"status"`
	NATType string `json:"nat_type,omitempty"`
	Error   string `json:"error,omitempty"`
}

// NATProbeEvidence stores the STUN-based NAT classification (gostun,
// Apache-2.0): cone/symmetric type, mapping/filtering behavior, port
// preservation, and hairpin capability. It supplements the conservative
// address-comparison heuristic in NATHeuristic and never gates the section.
type NATProbeEvidence struct {
	Status            string            `json:"status"` // ok | partial | error | unsupported
	NATType           string            `json:"nat_type,omitempty"`
	MappingBehavior   string            `json:"mapping_behavior,omitempty"`
	FilteringBehavior string            `json:"filtering_behavior,omitempty"`
	PortPreservation  string            `json:"port_preservation,omitempty"`
	Hairpin           string            `json:"hairpin,omitempty"`
	Partial           bool              `json:"partial,omitempty"`
	Successful        int               `json:"successful,omitempty"`
	Failed            int               `json:"failed,omitempty"`
	Results           []NATServerResult `json:"results,omitempty"`
	Message           string            `json:"message,omitempty"`
}

// natProbeTimeout bounds the whole multi-server STUN classification.
const natProbeTimeout = 45 * time.Second

// natServerCount caps the servers tried per family so restricted networks
// fail fast instead of exhausting every candidate.
const natServerCount = 4

// ProbeNATType runs the gostun STUN classification for the requested family
// ("v4", "v6", or "dual"). UDP-restricted environments surface as a
// structured error evidence entry.
func ProbeNATType(ctx context.Context, ipVersion string) *NATProbeEvidence {
	family := "ipv4"
	servers := model.GetDefaultServers("ipv4")
	switch strings.ToLower(strings.TrimSpace(ipVersion)) {
	case "v6", "ipv6", "6":
		family = "ipv6"
		servers = model.GetDefaultServers("ipv6")
	case "dual", "both", "all":
		family = "both"
		servers = append(append([]string{}, model.GetDefaultServers("ipv4")...), model.GetDefaultServers("ipv6")...)
	}
	if len(servers) > natServerCount*2 {
		servers = servers[:natServerCount*2]
	}
	if ctx == nil {
		ctx = context.Background()
	}
	probeCtx, cancel := context.WithTimeout(ctx, natProbeTimeout)
	defer cancel()
	summary := stuncheck.ProbeNAT(probeCtx, stuncheck.ProbeConfig{
		Servers:       servers,
		IPVersion:     family,
		Timeout:       natProbeTimeout,
		MaxConcurrent: 3,
	})
	return natEvidenceFromSummary(&summary)
}

// natEvidenceFromSummary maps a gostun NATSummary onto the vmbench wire
// shape; mapping is pure so tests can drive it from fixtures.
func natEvidenceFromSummary(summary *stuncheck.NATSummary) *NATProbeEvidence {
	if summary == nil {
		return &NATProbeEvidence{Status: "error", Message: "no NAT summary"}
	}
	evidence := &NATProbeEvidence{
		Partial:    summary.Partial,
		Successful: summary.Successful,
		Failed:     summary.Failed,
	}
	for _, report := range summary.Results {
		item := NATServerResult{
			Server:  report.Server,
			Status:  string(report.Status),
			NATType: report.NATType,
			Error:   report.Error,
		}
		evidence.Results = append(evidence.Results, item)
		if evidence.NATType == "" && report.NATType != "" && strings.EqualFold(string(report.Status), string(stuncheck.CapabilityAvailable)) {
			evidence.NATType = report.NATType
			evidence.MappingBehavior = report.MappingBehavior
			evidence.FilteringBehavior = report.FilteringBehavior
			evidence.PortPreservation = string(report.PortPreservation)
			evidence.Hairpin = string(report.Hairpin)
		}
	}
	switch {
	case summary.Successful > 0 && summary.Failed == 0:
		evidence.Status = "ok"
	case summary.Successful > 0:
		evidence.Status = "partial"
	case strings.TrimSpace(summary.Error) != "":
		evidence.Status = "error"
		evidence.Message = summary.Error
	default:
		evidence.Status = "unsupported"
		evidence.Message = "no STUN server produced a conclusive response"
	}
	if evidence.NATType == "" && evidence.Status != "error" {
		evidence.NATType = "Inconclusive"
	}
	if evidence.MappingBehavior == "" && summary.MappingConsistency != "" {
		evidence.MappingBehavior = string(summary.MappingConsistency)
	}
	if evidence.PortPreservation == "" && summary.PortPreservationConsistency != "" {
		evidence.PortPreservation = string(summary.PortPreservationConsistency)
	}
	if evidence.Hairpin == "" && summary.HairpinConsistency != "" {
		evidence.Hairpin = string(summary.HairpinConsistency)
	}
	return evidence
}
