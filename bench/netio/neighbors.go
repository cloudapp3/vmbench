package netio

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/oneclickvirt/basics/network/baseinfo"
	"github.com/oneclickvirt/basics/network/ipv6"
)

// CIDRNeighborsEvidence stores the bgp.tools prefix-map estimate of how many
// addresses in the local /24 subnet and the announced CIDR were observed
// active on the global monitoring network (refreshed upstream every 15-20
// minutes). It is a coarse activity hint, not a pingable-host count, and
// never gates the network identity section.
type CIDRNeighborsEvidence struct {
	Status          string `json:"status"` // ok | partial | error | unsupported
	IPv4            string `json:"ipv4,omitempty"`
	SubnetPrefix    string `json:"subnet_prefix,omitempty"` // e.g. 96.9.228.0/24
	SubnetActive    int    `json:"subnet_active,omitempty"`
	SubnetTotal     int    `json:"subnet_total,omitempty"`
	AnnouncedPrefix string `json:"announced_prefix,omitempty"` // e.g. 96.9.228.0/23
	PrefixActive    int    `json:"prefix_active,omitempty"`
	PrefixTotal     int    `json:"prefix_total,omitempty"`
	Message         string `json:"message,omitempty"`
}

// IPv6SubnetInfo stores the observed on-link IPv6 prefix length (RA / ip
// command / config fallback via basics, Apache-2.0). A /80 or larger block
// is what allows independent sub-allocation; the raw value is reported
// without recommendation.
type IPv6SubnetInfo struct {
	Status       string `json:"status"` // ok | error | unsupported
	Address      string `json:"address,omitempty"`
	PrefixLength int    `json:"prefix_length,omitempty"` // 64, 80, 128 ...
	Message      string `json:"message,omitempty"`
}

// neighborProbeTimeout bounds the bgp.tools prefix-map fetches and the IPv6
// prefix discovery, neither of which observes the caller context.
const neighborProbeTimeout = 20 * time.Second

var ipv6MaskValueRe = regexp.MustCompile(`/(\d+)`)

// ProbeCIDRNeighbors estimates active neighbor counts for the local /24 and
// the announced CIDR via the bgp.tools prefix-map (basics baseinfo).
func ProbeCIDRNeighbors(ctx context.Context, ipv4 string) *CIDRNeighborsEvidence {
	evidence := &CIDRNeighborsEvidence{IPv4: strings.TrimSpace(ipv4)}
	if evidence.IPv4 == "" || !strings.Contains(evidence.IPv4, ".") {
		evidence.Status = "unsupported"
		evidence.Message = "no public IPv4 to query"
		return evidence
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, neighborProbeTimeout)
	defer cancel()
	type neighborResult struct {
		prefix   string
		active   int
		total    int
		err      error
		isPrefix bool
	}
	results := make(chan neighborResult, 2)
	subnetBase := baseinfo.MaskIP(evidence.IPv4)
	run := func(prefix string, length int, isPrefix bool) {
		active, total, err := withNeighborTimeout(ctx, func() (int, int, error) {
			return baseinfo.GetActiveIpsCount(prefix, length)
		})
		results <- neighborResult{prefix: prefix + "/" + strconv.Itoa(length), active: active, total: total, err: err, isPrefix: isPrefix}
	}
	go run(subnetBase, 24, false)
	announcedBase, announcedLen := withCIDRPrefix(ctx, evidence.IPv4)
	// The announced CIDR is only an extra query when it differs from the
	// local /24; identical prefixes would just duplicate the same fetch.
	sameAsSubnet := announcedLen == 24 && announcedBase == subnetBase
	if announcedLen > 0 && announcedLen <= 24 && !sameAsSubnet {
		go run(announcedBase, announcedLen, true)
	}
	awaited := 1
	if announcedLen > 0 && announcedLen <= 24 && !sameAsSubnet {
		awaited = 2
	}
	failures := make([]string, 0, 2)
	for i := 0; i < awaited; i++ {
		result := <-results
		switch {
		case result.err != nil || result.active <= 0:
			if result.err != nil {
				failures = append(failures, result.prefix+": "+result.err.Error())
			} else {
				failures = append(failures, result.prefix+": no active addresses observed")
			}
		case result.isPrefix:
			evidence.AnnouncedPrefix = result.prefix
			evidence.PrefixActive, evidence.PrefixTotal = result.active, result.total
		default:
			evidence.SubnetPrefix = result.prefix
			evidence.SubnetActive, evidence.SubnetTotal = result.active, result.total
		}
	}
	switch {
	case evidence.SubnetActive > 0 && (announcedLen <= 0 || evidence.PrefixActive > 0 || announcedLen == 24):
		evidence.Status = "ok"
	case evidence.SubnetActive > 0 || evidence.PrefixActive > 0:
		evidence.Status = "partial"
	case len(failures) > 0:
		evidence.Status = "error"
		evidence.Message = strings.Join(failures, "; ")
	default:
		evidence.Status = "unsupported"
		evidence.Message = "no neighbor data returned"
	}
	return evidence
}

// ProbeIPv6Subnet discovers the on-link IPv6 prefix length for the observed
// public address (RA / ip command / config fallback, Apache-2.0 basics).
func ProbeIPv6Subnet(ctx context.Context, publicIPv6 string) *IPv6SubnetInfo {
	info := &IPv6SubnetInfo{Address: strings.TrimSpace(publicIPv6)}
	if info.Address == "" || !strings.Contains(info.Address, ":") {
		info.Status = "unsupported"
		info.Message = "no public IPv6 observed"
		return info
	}
	if ctx == nil {
		ctx = context.Background()
	}
	maskCtx, cancel := context.WithTimeout(ctx, neighborProbeTimeout)
	defer cancel()
	maskText, err := withIPv6MaskTimeout(maskCtx, info.Address)
	if err != nil {
		info.Status = "error"
		info.Message = err.Error()
		return info
	}
	match := ipv6MaskValueRe.FindStringSubmatch(maskText)
	if len(match) != 2 {
		info.Status = "error"
		info.Message = "no prefix length in discovery output"
		return info
	}
	length, parseErr := strconv.Atoi(match[1])
	if parseErr != nil {
		info.Status = "error"
		info.Message = parseErr.Error()
		return info
	}
	info.PrefixLength = length
	info.Status = "ok"
	return info
}

// withNeighborTimeout wraps the context-unaware bgp.tools fetches.
func withNeighborTimeout(ctx context.Context, probe func() (int, int, error)) (int, int, error) {
	type outcome struct {
		active int
		total  int
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		active, total, err := probe()
		done <- outcome{active, total, err}
	}()
	select {
	case result := <-done:
		return result.active, result.total, result.err
	case <-ctx.Done():
		return 0, 0, ctx.Err()
	}
}

func withCIDRPrefix(ctx context.Context, ip string) (string, int) {
	type outcome struct {
		base   string
		length int
	}
	done := make(chan outcome, 1)
	go func() {
		base, length := baseinfo.GetCIDRPrefix(ip)
		done <- outcome{base, length}
	}()
	select {
	case result := <-done:
		return result.base, result.length
	case <-ctx.Done():
		return "", 0
	}
}

func withIPv6MaskTimeout(ctx context.Context, address string) (string, error) {
	done := make(chan struct {
		text string
		err  error
	}, 1)
	go func() {
		text, err := ipv6.GetIPv6Mask(address, "en")
		done <- struct {
			text string
			err  error
		}{text, err}
	}()
	select {
	case result := <-done:
		return result.text, result.err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}
