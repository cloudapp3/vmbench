package netio

import "strings"

// RouteClassification is the conservative China-carrier return-route
// classification derived from vmbench's own HopASN evidence. Code and
// Confidence are stable machine-readable values; Label preserves the compact
// display used by console, HTML, and TUI.
type RouteClassification struct {
	Code       string `json:"code"`
	Label      string `json:"label"`
	Confidence string `json:"confidence"`
	Rank       int    `json:"rank"`
	Evidence   string `json:"evidence"`
}

const (
	routeConfidenceConfirmed    = "confirmed"
	routeConfidenceMixed        = "mixed"
	routeConfidenceInconclusive = "inconclusive"
)

// Route classification ranks: 5 marks a premium line, 3 a quality line,
// 2 a confirmed ordinary backbone, 0 inconclusive evidence. Renderers grade
// colors from these bands (checkup.RouteLineTone).
const (
	routeRankPremium  = 5
	routeRankQuality  = 3
	routeRankOrdinary = 2
	routeRankNone     = 0
)

// classifyReturnRoute classifies one trace's return route from the same
// HopASN evidence that annotates hops, so classification and hop annotations
// can never disagree (the historical bug: a second ASN database inside the
// backtrace library labeled CMI prefixes AS58807 while hops read AS58453,
// producing "CMIN2 confirmed" verdicts the hops did not support).
//
// Premium-line claims additionally require carriage hops: premium-ASN hops
// that are followed later in the trace by a hop annotated with a different
// carrier ASN. Premium-ASN hops appearing only as the trailing run before a
// destination inside that same network are destination-network delivery, not
// backbone carriage — a trace toward a CN2-hosted node always ends on 59.43
// hops regardless of how the machine's traffic actually enters China.
func classifyReturnRoute(carrier string, hops []Hop) RouteClassification {
	switch normalizeRouteCarrier(carrier) {
	case "CT":
		return classifyTelecomRoute(hops)
	case "CU":
		return classifyUnicomRoute(hops)
	case "CM":
		return classifyMobileRoute(hops)
	default:
		return inconclusiveRoute("unknown_carrier", "线路证据不足", "unknown carrier")
	}
}

func classifyTelecomRoute(hops []Hop) RouteClassification {
	evidence := newRouteHopEvidence(hops)
	cn2Total := evidence.count("AS4809")
	cn2Carriage := evidence.carriageCount("AS4809")
	ct163Total := evidence.count("AS4134")
	if cn2Carriage >= 2 {
		return RouteClassification{
			Code: "ct_cn2_gia", Label: "电信CN2GIA [精品线路]", Confidence: routeConfidenceConfirmed, Rank: routeRankPremium,
			Evidence: "at least two AS4809 hops are carried ahead of other backbone hops",
		}
	}
	if cn2Total > 0 && evidence.first("AS4134") >= 0 && evidence.first("AS4134") < evidence.first("AS4809") {
		return RouteClassification{
			Code: "ct_cn2_gt", Label: "电信CN2GT  [优质线路]", Confidence: routeConfidenceMixed, Rank: routeRankQuality,
			Evidence: "AS4134 appears before the AS4809 segment",
		}
	}
	if cn2Total > 0 {
		return RouteClassification{
			Code: "ct_cn2_mixed", Label: "电信CN2混合 [优质线路]", Confidence: routeConfidenceMixed, Rank: routeRankQuality,
			Evidence: "AS4809 appears only as trailing destination-network hops; CN2 carriage unconfirmed",
		}
	}
	if evidence.count("AS23764") > 0 {
		return RouteClassification{
			Code: "ct_ctgnet", Label: "电信CTGNET [普通线路]", Confidence: routeConfidenceConfirmed, Rank: routeRankOrdinary,
			Evidence: "AS23764 international handoff without domestic CN2/163 backbone evidence; ordinary CT international transit",
		}
	}
	if ct163Total >= 2 {
		return RouteClassification{
			Code: "ct_163", Label: "电信163    [普通线路]", Confidence: routeConfidenceConfirmed, Rank: routeRankOrdinary,
			Evidence: "multiple AS4134 hops are present without premium backbone evidence",
		}
	}
	if ct163Total == 1 {
		return inconclusiveRoute("ct_destination_only", "仅见电信目的网", "only one AS4134 hop")
	}
	return inconclusiveRoute("ct_unknown", "未见电信骨干", "AS4809, AS23764, and AS4134 are absent")
}

func classifyUnicomRoute(hops []Hop) RouteClassification {
	evidence := newRouteHopEvidence(hops)
	cu9929Carriage := evidence.carriageCount("AS9929")
	cu9929Total := evidence.count("AS9929")
	if cu9929Carriage >= 2 {
		return RouteClassification{
			Code: "cu_9929", Label: "联通9929   [优质线路]", Confidence: routeConfidenceConfirmed, Rank: routeRankQuality,
			Evidence: "at least two AS9929 hops are carried ahead of other backbone hops",
		}
	}
	if cu9929Total > 0 {
		if evidence.first("AS4837") >= 0 && evidence.first("AS4837") < evidence.first("AS9929") {
			return RouteClassification{
				Code: "cu_9929_mixed", Label: "联通9929混合 [优质线路]", Confidence: routeConfidenceMixed, Rank: routeRankQuality,
				Evidence: "AS4837 appears before the AS9929 segment",
			}
		}
		return RouteClassification{
			Code: "cu_9929_mixed", Label: "联通9929混合 [优质线路]", Confidence: routeConfidenceMixed, Rank: routeRankQuality,
			Evidence: "AS9929 appears only as trailing destination-network hops; 9929 carriage unconfirmed",
		}
	}
	if evidence.count("AS10099") > 0 {
		return RouteClassification{
			Code: "cu_cug", Label: "联通CUG    [普通线路]", Confidence: routeConfidenceConfirmed, Rank: routeRankOrdinary,
			Evidence: "AS10099 international handoff without AS9929; ordinary Unicom international transit",
		}
	}
	if n := evidence.count("AS4837"); n >= 2 {
		return RouteClassification{
			Code: "cu_4837", Label: "联通4837   [普通线路]", Confidence: routeConfidenceConfirmed, Rank: routeRankOrdinary,
			Evidence: "multiple AS4837 hops are present without premium backbone evidence",
		}
	} else if n == 1 {
		return inconclusiveRoute("cu_destination_only", "仅见联通目的网", "only one AS4837 hop")
	}
	return inconclusiveRoute("cu_unknown", "未见联通骨干", "AS9929, AS10099, and AS4837 are absent")
}

func classifyMobileRoute(hops []Hop) RouteClassification {
	evidence := newRouteHopEvidence(hops)
	cmin2Carriage := evidence.carriageCount("AS58807")
	cmin2Total := evidence.count("AS58807")
	if cmin2Carriage >= 1 {
		return RouteClassification{
			Code: "cm_cmin2", Label: "移动CMIN2  [精品线路]", Confidence: routeConfidenceConfirmed, Rank: routeRankPremium,
			Evidence: "AS58807 hops are carried ahead of other backbone hops",
		}
	}
	if cmin2Total > 0 {
		return RouteClassification{
			Code: "cm_cmin2_mixed", Label: "移动CMIN2混合 [优质线路]", Confidence: routeConfidenceMixed, Rank: routeRankQuality,
			Evidence: "AS58807 appears only as trailing destination-network hops; CMIN2 carriage unconfirmed",
		}
	}
	if evidence.count("AS58453") > 0 {
		return RouteClassification{
			Code: "cm_cmi", Label: "移动CMI    [普通线路]", Confidence: routeConfidenceConfirmed, Rank: routeRankOrdinary,
			Evidence: "AS58453 is present without CMIN2 evidence",
		}
	}
	if evidence.count("AS9808") > 0 {
		return RouteClassification{
			Code: "cm_cmnet", Label: "移动CMNET  [普通线路]", Confidence: routeConfidenceConfirmed, Rank: routeRankOrdinary,
			Evidence: "AS9808 is present without international premium backbone evidence",
		}
	}
	return inconclusiveRoute("cm_unknown", "未见移动骨干", "AS58807, AS58453, and AS9808 are absent")
}

// routeHopEvidence is the ordered list of carrier-annotated hops of one trace.
type routeHopEvidence struct {
	asns []string // one entry per responding hop; "" marks an unannotated hop
}

func newRouteHopEvidence(hops []Hop) routeHopEvidence {
	asns := make([]string, 0, len(hops))
	for _, hop := range hops {
		if hop.Timeout || strings.TrimSpace(hop.IP) == "" {
			continue
		}
		asns = append(asns, HopASN(hop.IP))
	}
	return routeHopEvidence{asns: asns}
}

// count returns how many annotated hops carry the target ASN.
func (e routeHopEvidence) count(asn string) int {
	total := 0
	for _, value := range e.asns {
		if value == asn {
			total++
		}
	}
	return total
}

// first returns the annotated-hop index of the first hop carrying the ASN,
// or -1 when absent.
func (e routeHopEvidence) first(asn string) int {
	for index, value := range e.asns {
		if value == asn {
			return index
		}
	}
	return -1
}

// carriageCount counts hops of the ASN that are followed later by a hop
// annotated with a different carrier ASN: evidence the ASN carried traffic
// onward instead of merely delivering it into the destination network.
func (e routeHopEvidence) carriageCount(asn string) int {
	carriage := 0
	for index, value := range e.asns {
		if value != asn {
			continue
		}
		for _, later := range e.asns[index+1:] {
			if later != "" && later != asn {
				carriage++
				break
			}
		}
	}
	return carriage
}

// observedRouteASNs returns the unique annotated hop ASNs in hop order.
func observedRouteASNs(hops []Hop) []string {
	seen := make(map[string]struct{}, 8)
	out := make([]string, 0, 8)
	for _, hop := range hops {
		if hop.Timeout || strings.TrimSpace(hop.IP) == "" {
			continue
		}
		asn := HopASN(hop.IP)
		if asn == "" {
			continue
		}
		if _, ok := seen[asn]; ok {
			continue
		}
		seen[asn] = struct{}{}
		out = append(out, asn)
	}
	return out
}

func normalizeRouteCarrier(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	switch value {
	case "CT", "TELECOM", "电信":
		return "CT"
	case "CU", "UNICOM", "联通":
		return "CU"
	case "CM", "CMCC", "MOBILE", "移动":
		return "CM"
	default:
		return value
	}
}

func inconclusiveRoute(code, label, evidence string) RouteClassification {
	return RouteClassification{
		Code: code, Label: label, Confidence: routeConfidenceInconclusive,
		Rank: routeRankNone, Evidence: evidence,
	}
}
