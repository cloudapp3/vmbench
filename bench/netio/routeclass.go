package netio

import (
	"context"
	"net"
	"strings"
)

// ClassifyTraceResults enriches already-collected traceroute results with a
// per-target return-route line classification. Evidence comes from the same
// HopASN carrier-prefix table that annotates hops, so classification verdicts
// and hop annotations can never disagree. No additional packets are sent and
// no raw-socket/ICMP privileges are required.
//
// Classification is best-effort evidence: results without hops or a resolved
// target are left untouched.
func ClassifyTraceResults(ctx context.Context, results []TraceProbeResult) {
	_ = ctx // classification is CPU-only over already-collected hops
	for i := range results {
		result := &results[i]
		if len(result.Hops) == 0 {
			continue
		}
		resolved := strings.TrimSpace(result.ResolvedTarget)
		if resolved == "" || net.ParseIP(resolved) == nil {
			continue
		}
		annotateHopASNs(result)
		classification := classifyReturnRoute(result.Target.Carrier, result.Hops)
		result.Classification = &classification
		result.ObservedASNs = observedRouteASNs(result.Hops)
	}
}

// annotateHopASNs labels each responding hop with a China carrier backbone
// ASN where the address matches a known carrier prefix.
func annotateHopASNs(result *TraceProbeResult) {
	for i := range result.Hops {
		if result.Hops[i].Timeout || result.Hops[i].IP == "" {
			continue
		}
		result.Hops[i].ASN = HopASN(result.Hops[i].IP)
	}
}
