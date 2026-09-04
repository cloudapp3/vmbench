package netio

import (
	"context"
	"net"
	"strings"
	"time"

	backtrace "github.com/oneclickvirt/backtrace/bk"
)

// RouteClassification is the conservative China-carrier line classification
// computed by the backtrace library (163 / 9929 / 4837 / CN2GIA / CN2GT /
// CTGNET / CMIN2 / CMI style codes with confidence and evidence).
type RouteClassification = backtrace.RouteClassification

// classifyTraceTimeout bounds the classification pass. Hops are already
// collected, so the pass itself is CPU-only; the budget only guards against
// unexpected stalls inside the library.
const classifyTraceTimeout = 30 * time.Second

// ClassifyTraceResults enriches already-collected traceroute results with a
// per-target return-route line classification. It drives backtrace's
// RunRouteReport with a cached-hop TraceFunc so no additional packets are
// sent and no raw-socket/ICMP privileges are required: the evidence comes
// from the system-traceroute hops stored in each result.
//
// Classification is best-effort evidence: failures leave results untouched.
func ClassifyTraceResults(ctx context.Context, results []TraceProbeResult) {
	defer func() { _ = recover() }()

	type pending struct {
		indexes []int
		hops    []Hop
		carrier string
		name    string
	}
	byAddress := make(map[string]*pending)
	order := make([]string, 0)
	for i := range results {
		result := &results[i]
		if len(result.Hops) == 0 {
			continue
		}
		ip := net.ParseIP(strings.TrimSpace(result.ResolvedTarget))
		if ip == nil {
			continue
		}
		address := ip.String()
		entry, ok := byAddress[address]
		if !ok {
			entry = &pending{carrier: result.Target.Carrier, name: result.Target.Name}
			byAddress[address] = entry
			order = append(order, address)
		}
		entry.indexes = append(entry.indexes, i)
		entry.hops = result.Hops
	}
	if len(order) == 0 {
		return
	}

	targets := make([]backtrace.RouteTarget, 0, len(order))
	for _, address := range order {
		entry := byAddress[address]
		ip := net.ParseIP(address)
		family := "v4"
		if ip.To4() == nil {
			family = "v6"
		}
		targets = append(targets, backtrace.RouteTarget{
			Name:      entry.name,
			Address:   address,
			IPVersion: family,
			Carrier:   entry.carrier,
		})
	}

	if ctx == nil {
		ctx = context.Background()
	}
	classifyCtx, cancel := context.WithTimeout(ctx, classifyTraceTimeout)
	defer cancel()

	report := backtrace.RunRouteReport(classifyCtx, backtrace.RouteReportConfig{
		Attempts: 1,
		Timeout:  classifyTraceTimeout,
		Targets:  targets,
		Trace: func(ctx context.Context, ip net.IP) ([]*backtrace.Hop, error) {
			entry, ok := byAddress[ip.String()]
			if !ok {
				return nil, &net.OpError{Op: "classify", Err: errUnknownTraceTarget}
			}
			return toBacktraceHops(entry.hops), nil
		},
		// Alternative targets would trigger extra probes; evidence is cached.
		AlternativeTarget: func(backtrace.RouteTarget) []string { return nil },
	})

	for _, targetReport := range report.Targets {
		entry, ok := byAddress[targetReport.Target.Address]
		if !ok {
			continue
		}
		classification := targetReport.Classification
		asns := targetReport.ObservedASNs
		for _, index := range entry.indexes {
			results[index].Classification = &classification
			if len(asns) > 0 {
				results[index].ObservedASNs = append([]string(nil), asns...)
			}
		}
	}
}

// errUnknownTraceTarget marks a cache miss inside the classification pass.
var errUnknownTraceTarget = &classificationError{"no cached hops for target"}

type classificationError struct{ message string }

func (e *classificationError) Error() string { return e.message }

// toBacktraceHops converts collected traceroute hops into backtrace Hop
// evidence. Timed-out hops carry no address and are dropped.
func toBacktraceHops(hops []Hop) []*backtrace.Hop {
	out := make([]*backtrace.Hop, 0, len(hops))
	for _, hop := range hops {
		if hop.Timeout || strings.TrimSpace(hop.IP) == "" {
			continue
		}
		ip := net.ParseIP(strings.TrimSpace(hop.IP))
		if ip == nil {
			continue
		}
		node := &backtrace.Node{IP: ip}
		if hop.RTTMs > 0 {
			node.RTT = []time.Duration{time.Duration(hop.RTTMs * float64(time.Millisecond))}
		}
		out = append(out, &backtrace.Hop{Distance: hop.TTL, Nodes: []*backtrace.Node{node}})
	}
	return out
}
