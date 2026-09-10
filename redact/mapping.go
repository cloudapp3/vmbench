package redact

import (
	"encoding/json"
	"fmt"
	"net"
	"sort"
	"strings"
)

// documentationNetworks are the reserved example ranges (RFC 5737 / RFC 3849).
// The placeholders themselves live there, so never redacting inside them
// keeps Apply idempotent and existing test fixtures intact.
var documentationNetworks = mustParseNetworks(
	"192.0.2.0/24", "198.51.100.0/24", "203.0.113.0/24", "2001:db8::/32",
)

func mustParseNetworks(values ...string) []net.IPNet {
	out := make([]net.IPNet, 0, len(values))
	for _, value := range values {
		_, network, err := net.ParseCIDR(value)
		if err != nil {
			panic("redact: invalid documentation network " + value)
		}
		out = append(out, *network)
	}
	return out
}

// harvestPublicIPs walks the report JSON tree and collects the machine's
// public addresses from the identity fields filled by the what-is-my-ip
// probes. Every other occurrence (free text, prefixes, reverse DNS) refers
// back to these authoritative fields.
func harvestPublicIPs(raw []byte) []net.IP {
	var tree any
	if err := json.Unmarshal(raw, &tree); err != nil {
		return nil
	}
	root, ok := tree.(map[string]any)
	if !ok {
		return nil
	}
	var out []net.IP
	if result := digObject(digObject(root, "network_info"), "result"); result != nil {
		for _, key := range []string{"public_ipv4", "public_ipv6"} {
			out = appendIP(out, digString(digObject(result, key), "ip"))
		}
		for _, item := range digList(result, "nat") {
			out = appendIP(out, digString(asObject(item), "public_ip"))
		}
		// Interface addresses are the same machine's: SLAAC assigns several
		// global v6 addresses per interface and only one is the observed
		// egress address. Private entries are filtered later by isRedactable.
		for _, item := range digList(result, "local_global_addresses") {
			out = appendIP(out, digString(asObject(item), "address"))
		}
		out = appendIP(out, digString(digObject(result, "cidr_neighbors"), "ipv4"))
		out = appendIP(out, digString(digObject(result, "ipv6_subnet"), "address"))
		out = appendIP(out, digString(digObject(result, "ip_bgp"), "ip"))
	}
	if result := digObject(digObject(root, "ip_quality"), "result"); result != nil {
		out = appendIP(out, digString(digObject(result, "basic_info"), "ip"))
		out = appendIP(out, digString(digObject(result, "ipapi_is"), "ip"))
	}
	return out
}

func digObject(node map[string]any, key string) map[string]any {
	if node == nil {
		return nil
	}
	return asObject(node[key])
}

func asObject(value any) map[string]any {
	object, _ := value.(map[string]any)
	return object
}

func digList(node map[string]any, key string) []any {
	if node == nil {
		return nil
	}
	list, _ := node[key].([]any)
	return list
}

func digString(node map[string]any, key string) string {
	if node == nil {
		return ""
	}
	text, _ := node[key].(string)
	return text
}

func appendIP(out []net.IP, value string) []net.IP {
	if ip := net.ParseIP(strings.TrimSpace(value)); ip != nil {
		return append(out, ip)
	}
	return out
}

// isRedactable reports whether ip should be masked: a usable public address
// outside the documentation ranges.
func isRedactable(ip net.IP) bool {
	if ip == nil || !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() ||
		ip.IsLinkLocalUnicast() || ip.IsMulticast() || ip.IsUnspecified() {
		return false
	}
	for _, network := range documentationNetworks {
		if network.Contains(ip) {
			return false
		}
	}
	return true
}

// mappingEntry pairs one original address with its placeholder.
type mappingEntry struct {
	original net.IP
	masked   string
	// reverseDNS is the leading "d.c.b.a." label of DNSBL lookup names,
	// with its masked counterpart; both stay empty for IPv6 because the
	// probes only build reverse names for IPv4.
	reverseDNS       string
	maskedReverseDNS string
}

// addressMapping assigns stable placeholders: IPv4 addresses sort before
// IPv6, each family in canonical string order, and receive
// 203.0.113.1, 203.0.113.2, ... and 2001:db8::1, 2001:db8::2, ... in turn.
type addressMapping struct {
	entries []mappingEntry
}

func newMapping(addresses []net.IP) *addressMapping {
	seen := make(map[string]struct{}, len(addresses))
	var v4, v6 []net.IP
	for _, ip := range addresses {
		if !isRedactable(ip) {
			continue
		}
		key := ip.String()
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		if ip.To4() != nil {
			v4 = append(v4, ip)
		} else {
			v6 = append(v6, ip)
		}
	}
	sort.Slice(v4, func(i, j int) bool { return v4[i].String() < v4[j].String() })
	sort.Slice(v6, func(i, j int) bool { return v6[i].String() < v6[j].String() })

	mapping := &addressMapping{}
	for idx, ip := range v4 {
		masked := fmt.Sprintf("203.0.113.%d", documentationHost(idx+1))
		mapping.entries = append(mapping.entries, mappingEntry{
			original:         ip,
			masked:           masked,
			reverseDNS:       reverseDNSLabel(ip),
			maskedReverseDNS: reverseDNSLabel(net.ParseIP(masked)),
		})
	}
	for idx, ip := range v6 {
		mapping.entries = append(mapping.entries, mappingEntry{
			original: ip,
			masked:   fmt.Sprintf("2001:db8::%x", idx+1),
		})
	}
	return mapping
}

// documentationHost keeps the TEST-NET-3 host part inside 1-254. More than a
// handful of distinct addresses never occurs (the probes observe at most one
// per family per section); clamping only guards the theoretical overflow.
func documentationHost(index int) int {
	if index > 254 {
		return 254
	}
	return index
}

// reverseDNSLabel returns the reversed dotted quad with a trailing dot, as
// used to build DNSBL query names ("1.2.3.4" → "4.3.2.1.").
func reverseDNSLabel(ip net.IP) string {
	if ip == nil || ip.To4() == nil {
		return ""
	}
	octets := strings.Split(ip.To4().String(), ".")
	for i, j := 0, len(octets)-1; i < j; i, j = i+1, j-1 {
		octets[i], octets[j] = octets[j], octets[i]
	}
	return strings.Join(octets, ".") + "."
}

func (m *addressMapping) mappedLists() (v4, v6 []string) {
	for _, entry := range m.entries {
		if entry.original.To4() != nil {
			v4 = append(v4, entry.original.String())
		} else {
			v6 = append(v6, entry.original.String())
		}
	}
	return v4, v6
}
