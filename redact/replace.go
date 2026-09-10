package redact

import (
	"bytes"
	"fmt"
	"net"
	"regexp"
	"sort"
	"strings"
)

// Candidate token finders. They only locate plausible spans; net.ParseCIDR
// and range parsing validate them before anything is replaced.
var (
	v4CIDRToken  = regexp.MustCompile(`(?:\d{1,3}\.){3}\d{1,3}/\d{1,3}`)
	v6CIDRToken  = regexp.MustCompile(`[0-9A-Fa-f:]*:[0-9A-Fa-f:]*/\d{1,3}`)
	v4RangeToken = regexp.MustCompile(`(?:\d{1,3}\.){3}\d{1,3} - (?:\d{1,3}\.){3}\d{1,3}`)
	v6RangeToken = regexp.MustCompile(`[0-9A-Fa-f:]*:[0-9A-Fa-f:]* - [0-9A-Fa-f:]*:[0-9A-Fa-f:]*`)
)

// replaceAll runs the three passes in load-bearing order: prefixes and
// ranges first (so a bare replacement cannot corrupt a CIDR that embeds the
// address), then reverse DNSBL labels, then bare addresses.
func (m *addressMapping) replaceAll(raw []byte) ([]byte, int) {
	total := 0
	body, count := m.replaceNetworkTokens(raw)
	total += count
	for _, entry := range m.entries {
		if entry.reverseDNS == "" {
			continue
		}
		body, count = replaceBounded(body, entry.reverseDNS, entry.maskedReverseDNS)
		total += count
	}
	// Longest originals first so one address cannot match inside a longer
	// nearby token before its own turn; the boundary guard already prevents
	// partial overlaps, this is belt and braces.
	ordered := append([]mappingEntry(nil), m.entries...)
	sort.Slice(ordered, func(i, j int) bool {
		return len(ordered[i].original.String()) > len(ordered[j].original.String())
	})
	for _, entry := range ordered {
		body, count = replaceBounded(body, entry.original.String(), entry.masked)
		total += count
	}
	return body, total
}

// networkToken is one candidate prefix/range occurrence in the body.
type networkToken struct {
	start, end int
	text       string
}

type spanReplacement struct {
	start, end int
	text       string
}

func (m *addressMapping) replaceNetworkTokens(body []byte) ([]byte, int) {
	var repls []spanReplacement
	for _, token := range findNetworkTokens(body) {
		if masked := m.maskNetworkToken(token.text); masked != "" {
			repls = append(repls, spanReplacement{start: token.start, end: token.end, text: masked})
		}
	}
	if len(repls) == 0 {
		return body, 0
	}
	sort.Slice(repls, func(i, j int) bool { return repls[i].start < repls[j].start })
	var out bytes.Buffer
	out.Grow(len(body))
	prev := 0
	count := 0
	for _, repl := range repls {
		if repl.start < prev {
			continue // overlapping candidate; the earlier replacement won
		}
		out.Write(body[prev:repl.start])
		out.WriteString(repl.text)
		prev = repl.end
		count++
	}
	out.Write(body[prev:])
	return out.Bytes(), count
}

func findNetworkTokens(body []byte) []networkToken {
	var tokens []networkToken
	for _, pattern := range []*regexp.Regexp{v4CIDRToken, v6CIDRToken, v4RangeToken, v6RangeToken} {
		for _, loc := range pattern.FindAllIndex(body, -1) {
			start, end := loc[0], loc[1]
			if !tokenBoundaryOK(body, start, end) {
				continue
			}
			tokens = append(tokens, networkToken{start: start, end: end, text: string(body[start:end])})
		}
	}
	sort.Slice(tokens, func(i, j int) bool { return tokens[i].start < tokens[j].start })
	return tokens
}

// tokenBoundaryOK reports whether the candidate at [start, end) is a whole
// token. The leading byte must not continue an address (an octet like
// "6.9.228.0/24" cut out of "96.9.228.0/24", or a v4 quad embedded in a
// mapped v6 form). The trailing check only rejects digits: prefixes and
// ranges end in an address or mask, so only a longer number can extend the
// token, while punctuation like "…/24: invalid PNG" must stay eligible.
func tokenBoundaryOK(body []byte, start, end int) bool {
	if start > 0 && isAddressByte(body[start-1]) {
		return false
	}
	if end < len(body) && isDigit(body[end]) {
		return false
	}
	return true
}

func isDigit(b byte) bool {
	return b >= '0' && b <= '9'
}

func isAddressByte(b byte) bool {
	return b == '.' || b == ':' || (b >= '0' && b <= '9') ||
		(b >= 'a' && b <= 'f') || (b >= 'A' && b <= 'F')
}

// maskNetworkToken returns the masked form of one CIDR or "start - end"
// range token, or "" when the token does not embed any local address.
// Prefixes keep their length while it stays inside the documentation block
// (v4 ≥ /24, v6 ≥ /64) and collapse to that boundary otherwise, so the
// "a prefix exists" evidence survives without narrowing the real allocation.
func (m *addressMapping) maskNetworkToken(token string) string {
	if start, end, ok := splitAddressRange(token); ok {
		if m.rangeContainsLocal(start, end) {
			return m.rangePlaceholder(start)
		}
		return ""
	}
	ip, network, err := net.ParseCIDR(token)
	if err != nil {
		return ""
	}
	if !m.networkContainsLocal(network) {
		return ""
	}
	ones, bits := network.Mask.Size()
	if bits == 32 {
		switch {
		case ones >= 32:
			return m.maskedFor(ip) + "/32"
		case ones >= 24:
			return fmt.Sprintf("203.0.113.0/%d", ones)
		default:
			return "203.0.113.0/24"
		}
	}
	switch {
	case ones >= 128:
		return m.maskedFor(ip) + "/128"
	case ones >= 64:
		return fmt.Sprintf("2001:db8::/%d", ones)
	default:
		return "2001:db8::/64"
	}
}

func (m *addressMapping) networkContainsLocal(network *net.IPNet) bool {
	for _, entry := range m.entries {
		if network.Contains(entry.original) {
			return true
		}
	}
	return false
}

func (m *addressMapping) rangeContainsLocal(start, end net.IP) bool {
	for _, entry := range m.entries {
		ip := entry.original
		if (ip.To4() == nil) != (start.To4() == nil) {
			continue
		}
		if bytes.Compare(ip.To16(), start.To16()) >= 0 && bytes.Compare(ip.To16(), end.To16()) <= 0 {
			return true
		}
	}
	return false
}

func (m *addressMapping) rangePlaceholder(start net.IP) string {
	if start.To4() == nil {
		return "2001:db8::/64"
	}
	return "203.0.113.0/24"
}

// maskedFor returns the placeholder assigned to ip; unreachable without a
// local entry because every caller first confirms containment.
func (m *addressMapping) maskedFor(ip net.IP) string {
	for _, entry := range m.entries {
		if entry.original.Equal(ip) {
			return entry.masked
		}
	}
	return "203.0.113.254"
}

func splitAddressRange(token string) (net.IP, net.IP, bool) {
	left, right, ok := strings.Cut(token, " - ")
	if !ok {
		return nil, nil, false
	}
	start := net.ParseIP(strings.TrimSpace(left))
	end := net.ParseIP(strings.TrimSpace(right))
	if start == nil || end == nil || (start.To4() == nil) != (end.To4() == nil) {
		return nil, nil, false
	}
	return start, end, true
}

// replaceBounded replaces every occurrence of needle whose neighbouring
// bytes are not digits or dots, so "1.2.3.4" never matches inside
// "1.2.3.45" or "11.2.3.4" while still matching inside URLs
// ("http://1.2.3.4:8080") and mapped forms ("::ffff:1.2.3.4").
func replaceBounded(body []byte, needle, replacement string) ([]byte, int) {
	if needle == "" || len(needle) > len(body) {
		return body, 0
	}
	var positions []int
	needleBytes := []byte(needle)
	for offset := 0; ; {
		idx := bytes.Index(body[offset:], needleBytes)
		if idx < 0 {
			break
		}
		start := offset + idx
		end := start + len(needle)
		if boundedAt(body, start, end) {
			positions = append(positions, start)
		}
		offset = end
	}
	if len(positions) == 0 {
		return body, 0
	}
	out := make([]byte, 0, len(body))
	prev := 0
	for _, pos := range positions {
		out = append(out, body[prev:pos]...)
		out = append(out, replacement...)
		prev = pos + len(needle)
	}
	out = append(out, body[prev:]...)
	return out, len(positions)
}

func boundedAt(body []byte, start, end int) bool {
	if start > 0 && isDigitOrDot(body[start-1]) {
		return false
	}
	if end < len(body) && isDigitOrDot(body[end]) {
		return false
	}
	return true
}

func isDigitOrDot(b byte) bool {
	return b == '.' || (b >= '0' && b <= '9')
}
