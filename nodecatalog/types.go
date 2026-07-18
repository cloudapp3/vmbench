package nodecatalog

import (
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"
)

const (
	SchemaVersion = 1

	SourceEmbedded = "embedded"
	SourceAuto     = "auto"
	SourcePath     = "path"

	KindDownload  = "download"
	KindUpload    = "upload"
	KindRoute     = "route"
	KindPing      = "ping"
	KindRoutePing = "route_ping"
)

var nodeIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,127}$`)

// Manifest is a versioned, portable snapshot of benchmark network nodes.
type Manifest struct {
	SchemaVersion int       `json:"schema_version"`
	Revision      string    `json:"revision"`
	GeneratedAt   time.Time `json:"generated_at"`
	ExpiresAt     time.Time `json:"expires_at"`
	Nodes         []Node    `json:"nodes"`
}

// Node describes one network measurement endpoint. Fields which do not apply
// to a node kind remain empty or zero in the JSON manifest.
type Node struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Kind         string `json:"kind"`
	Region       string `json:"region"`
	City         string `json:"city"`
	Carrier      string `json:"carrier"`
	ASN          int    `json:"asn"`
	IPFamily     string `json:"ip_family"`
	Protocol     string `json:"protocol"`
	Endpoint     string `json:"endpoint"`
	Port         int    `json:"port"`
	URL          string `json:"url"`
	TrafficBytes int64  `json:"traffic_bytes"`
	Source       string `json:"source"`
}

// Filter selects nodes without changing manifest ordering.
type Filter struct {
	Kind     string
	IPFamily string
	Region   string
	City     string
	Carrier  string
}

// Validate checks the complete manifest contract before it is used or cached.
func (m Manifest) Validate() error {
	if m.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported node catalog schema_version %d (want %d)", m.SchemaVersion, SchemaVersion)
	}
	if strings.TrimSpace(m.Revision) == "" {
		return fmt.Errorf("node catalog revision is required")
	}
	if m.GeneratedAt.IsZero() {
		return fmt.Errorf("node catalog generated_at is required")
	}
	if m.ExpiresAt.IsZero() {
		return fmt.Errorf("node catalog expires_at is required")
	}
	if !m.ExpiresAt.After(m.GeneratedAt) {
		return fmt.Errorf("node catalog expires_at must be after generated_at")
	}
	if len(m.Nodes) == 0 {
		return fmt.Errorf("node catalog must contain at least one node")
	}
	if len(m.Nodes) > 4096 {
		return fmt.Errorf("node catalog contains too many nodes: %d", len(m.Nodes))
	}

	seen := make(map[string]struct{}, len(m.Nodes))
	for i, node := range m.Nodes {
		if err := node.Validate(); err != nil {
			return fmt.Errorf("node %d: %w", i, err)
		}
		if _, exists := seen[node.ID]; exists {
			return fmt.Errorf("node %d: duplicate id %q", i, node.ID)
		}
		seen[node.ID] = struct{}{}
	}
	return nil
}

// Validate checks fields and kind-specific requirements for one node.
func (n Node) Validate() error {
	if !nodeIDPattern.MatchString(n.ID) {
		return fmt.Errorf("invalid id %q", n.ID)
	}
	if strings.TrimSpace(n.Name) == "" {
		return fmt.Errorf("node %q name is required", n.ID)
	}
	switch n.Kind {
	case KindDownload, KindUpload, KindRoute, KindPing, KindRoutePing:
	default:
		return fmt.Errorf("node %q has unsupported kind %q", n.ID, n.Kind)
	}
	switch n.IPFamily {
	case "v4", "v6", "dual", "any":
	default:
		return fmt.Errorf("node %q has unsupported ip_family %q", n.ID, n.IPFamily)
	}
	switch n.Protocol {
	case "http", "https", "tcp", "dns":
	default:
		return fmt.Errorf("node %q has unsupported protocol %q", n.ID, n.Protocol)
	}
	if n.Port < 0 || n.Port > 65535 {
		return fmt.Errorf("node %q has invalid port %d", n.ID, n.Port)
	}
	if n.TrafficBytes < 0 {
		return fmt.Errorf("node %q has negative traffic_bytes", n.ID)
	}
	if strings.TrimSpace(n.Source) == "" {
		return fmt.Errorf("node %q source is required", n.ID)
	}

	switch n.Kind {
	case KindDownload, KindUpload:
		parsed, err := url.Parse(n.URL)
		if err != nil || parsed.Hostname() == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			return fmt.Errorf("node %q has invalid HTTP URL %q", n.ID, n.URL)
		}
		if n.Protocol != parsed.Scheme {
			return fmt.Errorf("node %q protocol %q does not match URL scheme %q", n.ID, n.Protocol, parsed.Scheme)
		}
		if n.Endpoint == "" {
			return fmt.Errorf("node %q endpoint is required", n.ID)
		}
		if n.TrafficBytes <= 0 {
			return fmt.Errorf("node %q traffic_bytes must be positive", n.ID)
		}
	case KindRoute, KindPing, KindRoutePing:
		if strings.TrimSpace(n.Endpoint) == "" {
			return fmt.Errorf("node %q endpoint is required", n.ID)
		}
		if n.Protocol != "tcp" && n.Protocol != "dns" {
			return fmt.Errorf("node %q route/ping protocol must be tcp or dns", n.ID)
		}
		if n.Protocol == "tcp" && n.Port == 0 {
			return fmt.Errorf("node %q TCP port is required", n.ID)
		}
	}
	return nil
}

// Clone returns a copy callers can mutate without changing a cached manifest.
func (m Manifest) Clone() Manifest {
	out := m
	out.Nodes = slices.Clone(m.Nodes)
	return out
}

// Select returns nodes matching all non-empty filters.
func (m Manifest) Select(filter Filter) []Node {
	out := make([]Node, 0, len(m.Nodes))
	for _, node := range m.Nodes {
		if !node.MatchesKind(filter.Kind) || !matchesFamily(node.IPFamily, filter.IPFamily) ||
			!matchesFold(node.Region, filter.Region) || !matchesFold(node.City, filter.City) ||
			!matchesFold(node.Carrier, filter.Carrier) {
			continue
		}
		out = append(out, node)
	}
	return out
}

// NodeIDs returns stable node identifiers for a report's selected catalog view.
func (m Manifest) NodeIDs(filter Filter) []string {
	nodes := m.Select(filter)
	ids := make([]string, 0, len(nodes))
	for _, node := range nodes {
		ids = append(ids, node.ID)
	}
	return ids
}

// MatchesKind treats route_ping nodes as members of both route and ping views.
func (n Node) MatchesKind(kind string) bool {
	kind = strings.ToLower(strings.TrimSpace(kind))
	if kind == "" || kind == "all" {
		return true
	}
	if n.Kind == kind {
		return true
	}
	return n.Kind == KindRoutePing && (kind == KindRoute || kind == KindPing)
}

func matchesFold(value, filter string) bool {
	filter = strings.TrimSpace(filter)
	return filter == "" || strings.EqualFold(strings.TrimSpace(value), filter)
}

func matchesFamily(value, filter string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	filter = strings.ToLower(strings.TrimSpace(filter))
	switch filter {
	case "", "any", "all":
		return true
	case "v4":
		return value == "v4" || value == "dual" || value == "any"
	case "v6":
		return value == "v6" || value == "dual" || value == "any"
	case "dual":
		return value == "v4" || value == "v6" || value == "dual" || value == "any"
	default:
		return false
	}
}
