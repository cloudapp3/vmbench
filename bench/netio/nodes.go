package netio

import (
	"strings"

	"github.com/cloudapp3/vmbench/nodecatalog"
)

// SpeedNode describes a public speed-test endpoint.
type SpeedNode struct {
	ID           string `json:"id,omitempty"`
	Name         string `json:"name"`
	Region       string `json:"region"`
	City         string `json:"city,omitempty"`
	Carrier      string `json:"carrier,omitempty"`
	ASN          int    `json:"asn,omitempty"`
	IPFamily     string `json:"ip_family,omitempty"`
	Protocol     string `json:"protocol,omitempty"`
	TestURL      string `json:"test_url"`
	TrafficBytes int64  `json:"traffic_bytes,omitempty"`
	Source       string `json:"source,omitempty"`
}

// DefaultNodes returns the built-in speed-test node list. It retains the old
// API while sourcing all values from the embedded versioned catalog.
func DefaultNodes() []SpeedNode {
	manifest, err := nodecatalog.Embedded()
	if err != nil {
		return nil
	}
	return SpeedNodesFromManifest(manifest)
}

// SpeedNodesFromManifest maps download nodes from a selected catalog.
func SpeedNodesFromManifest(manifest nodecatalog.Manifest) []SpeedNode {
	nodes := manifest.Select(nodecatalog.Filter{Kind: nodecatalog.KindDownload})
	out := make([]SpeedNode, 0, len(nodes))
	for _, node := range nodes {
		out = append(out, SpeedNode{
			ID:           node.ID,
			Name:         node.Name,
			Region:       node.Region,
			City:         node.City,
			Carrier:      node.Carrier,
			ASN:          node.ASN,
			IPFamily:     node.IPFamily,
			Protocol:     node.Protocol,
			TestURL:      node.URL,
			TrafficBytes: node.TrafficBytes,
			Source:       node.Source,
		})
	}
	return out
}

// TraceTargetsFromManifest maps route-capable nodes for one IP family.
func TraceTargetsFromManifest(manifest nodecatalog.Manifest, ipFamily string) []TraceTarget {
	nodes := manifest.Select(nodecatalog.Filter{Kind: nodecatalog.KindRoute})
	out := make([]TraceTarget, 0, len(nodes))
	for _, node := range nodes {
		if !matchesIPFamily(node.IPFamily, ipFamily) {
			continue
		}
		out = append(out, TraceTarget{
			ID:       node.ID,
			Name:     node.Name,
			Region:   node.Region,
			City:     node.City,
			Carrier:  node.Carrier,
			AS:       node.ASN,
			IPFamily: node.IPFamily,
			Protocol: node.Protocol,
			Endpoint: node.Endpoint,
			Port:     node.Port,
			Source:   node.Source,
		})
	}
	return out
}

// DefaultPingTargets returns catalog ping targets for v4, v6, or dual mode.
func DefaultPingTargets(ipFamily string) []PingTarget {
	manifest, err := nodecatalog.Embedded()
	if err != nil {
		return nil
	}
	return PingTargetsFromManifest(manifest, ipFamily)
}

// PingTargetsFromManifest maps ping-capable nodes for one IP family.
func PingTargetsFromManifest(manifest nodecatalog.Manifest, ipFamily string) []PingTarget {
	nodes := manifest.Select(nodecatalog.Filter{Kind: nodecatalog.KindPing})
	out := make([]PingTarget, 0, len(nodes))
	for _, node := range nodes {
		if !matchesIPFamily(node.IPFamily, ipFamily) {
			continue
		}
		out = append(out, PingTarget{
			ID:       node.ID,
			Name:     node.Name,
			Region:   node.Region,
			City:     node.City,
			Carrier:  node.Carrier,
			ASN:      node.ASN,
			IPFamily: node.IPFamily,
			Protocol: node.Protocol,
			Endpoint: node.Endpoint,
			Port:     node.Port,
			Source:   node.Source,
		})
	}
	return out
}

func matchesIPFamily(nodeFamily, requested string) bool {
	nodeFamily = strings.ToLower(strings.TrimSpace(nodeFamily))
	requested = strings.ToLower(strings.TrimSpace(requested))
	switch requested {
	case "", "v4", "ipv4":
		return nodeFamily == "v4" || nodeFamily == "dual" || nodeFamily == "any"
	case "v6", "ipv6":
		return nodeFamily == "v6" || nodeFamily == "dual" || nodeFamily == "any"
	case "dual", "all", "any":
		return true
	default:
		return false
	}
}
