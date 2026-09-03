package netio

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/cloudapp3/vmbench/bench"
	"github.com/cloudapp3/vmbench/nodecatalog"
)

const (
	pingProbes  = 10
	pingPort    = 80
	pingTimeout = 5 * time.Second

	// Connection-state values distinguish an accepted TCP connection from a
	// refused connection that still proves the target responded with a RST.
	PingConnectionStateOpen       = "open"
	PingConnectionStateRefused    = "refused"
	PingConnectionStateMixed      = "mixed"
	PingConnectionStateNoResponse = "no_response"
)

// PingProbeResult stores latency statistics for one probe target.
type PingProbeResult struct {
	ID              string  `json:"id,omitempty"`
	Name            string  `json:"name,omitempty"`
	Region          string  `json:"region,omitempty"`
	City            string  `json:"city,omitempty"`
	Carrier         string  `json:"carrier,omitempty"`
	ASN             int     `json:"asn,omitempty"`
	IPFamily        string  `json:"ip_family,omitempty"`
	Protocol        string  `json:"protocol,omitempty"`
	Source          string  `json:"source,omitempty"`
	ProbeProtocol   string  `json:"probe_protocol"`
	ProbeTool       string  `json:"probe_tool"`
	Target          string  `json:"target,omitempty"`
	Port            int     `json:"port,omitempty"`
	Status          string  `json:"status,omitempty"`
	ConnectionState string  `json:"connection_state,omitempty"`
	Message         string  `json:"message,omitempty"`
	AvgLatencyMs    float64 `json:"avg_latency_ms"`
	JitterMs        float64 `json:"jitter_ms"`
	PacketLoss      float64 `json:"packet_loss"`
	Sent            int     `json:"sent,omitempty"`
	Received        int     `json:"received,omitempty"`
}

type PingTarget struct {
	ID       string `json:"id,omitempty"`
	Name     string `json:"name"`
	Region   string `json:"region,omitempty"`
	City     string `json:"city,omitempty"`
	Carrier  string `json:"carrier,omitempty"`
	ASN      int    `json:"asn,omitempty"`
	IPFamily string `json:"ip_family,omitempty"`
	Protocol string `json:"protocol,omitempty"`
	Endpoint string `json:"endpoint"`
	Port     int    `json:"port,omitempty"`
	Source   string `json:"source,omitempty"`
}

type nodePingResult struct {
	name            string
	region          string
	host            string
	rtts            []time.Duration
	loss            float64
	connectionState string
	message         string
	err             error
}

type pingDialFunc func(context.Context, string, string) (net.Conn, error)

type tcpPingEvidence struct {
	rtts         []time.Duration
	rstResponses int
	lastErr      error
	lastRSTErr   error
}

// ProbePingNodes measures TCP connect latency to the provided speed nodes.
func ProbePingNodes(ctx context.Context, nodes []SpeedNode) ([]PingProbeResult, error) {
	return probePingNodes(ctx, nodes, pingNode)
}

func probePingNodes(ctx context.Context, nodes []SpeedNode, probe func(context.Context, SpeedNode) nodePingResult) ([]PingProbeResult, error) {
	results := make([]nodePingResult, len(nodes))
	var wg sync.WaitGroup
	for i, node := range nodes {
		wg.Add(1)
		go func(idx int, n SpeedNode) {
			defer wg.Done()
			results[idx] = probe(ctx, n)
		}(i, node)
	}
	wg.Wait()

	out := make([]PingProbeResult, 0, len(results))
	for _, r := range results {
		item := PingProbeResult{
			ID:              strings.ToLower(strings.ReplaceAll(r.name, " ", "-")),
			Name:            r.name,
			Region:          r.region,
			Protocol:        "tcp",
			ProbeProtocol:   "tcp-connect",
			ProbeTool:       "go-net-dialer",
			Target:          r.host,
			Port:            pingPort,
			ConnectionState: r.connectionState,
			PacketLoss:      r.loss,
			Sent:            pingProbes,
			Received:        len(r.rtts),
		}
		if len(r.rtts) == 0 {
			item.Status = "error"
			if r.err != nil {
				item.Message = r.err.Error()
			} else {
				item.Message = "no successful probes"
			}
		} else {
			item.Status = "ok"
			item.Message = r.message
			item.AvgLatencyMs = avgDuration(r.rtts).Seconds() * 1000
			item.JitterMs = jitterDuration(r.rtts).Seconds() * 1000
		}
		out = append(out, item)
	}
	return out, pingProbeError(ctx, out)
}

// ProbePingTargets measures TCP connect latency to the provided targets.
func ProbePingTargets(ctx context.Context, targets []PingTarget) ([]PingProbeResult, error) {
	return probePingTargets(ctx, targets, pingTarget)
}

func probePingTargets(ctx context.Context, targets []PingTarget, probe func(context.Context, PingTarget) PingProbeResult) ([]PingProbeResult, error) {
	results := make([]PingProbeResult, len(targets))
	var wg sync.WaitGroup
	for i, target := range targets {
		wg.Add(1)
		go func(idx int, t PingTarget) {
			defer wg.Done()
			results[idx] = probe(ctx, t)
		}(i, target)
	}
	wg.Wait()
	return results, pingProbeError(ctx, results)
}

func pingProbeError(ctx context.Context, results []PingProbeResult) error {
	if len(results) == 0 {
		return nil
	}
	firstMessage := ""
	for _, result := range results {
		if strings.EqualFold(strings.TrimSpace(result.Status), "ok") {
			return nil
		}
		if firstMessage == "" {
			firstMessage = strings.TrimSpace(result.Message)
		}
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("all %d ping targets failed: %w", len(results), err)
	}
	if firstMessage == "" {
		firstMessage = "no successful probes"
	}
	return fmt.Errorf("all %d ping targets failed: %s", len(results), firstMessage)
}

// pingWorkload measures TCP connect latency to all default nodes concurrently.
type pingWorkload struct {
	detail       string
	avgMs        float64
	lossPct      float64
	elapsed      time.Duration
	okCount      int64
	runErr       error
	targets      []PingTarget
	targetsSet   bool
	probeNodes   func(context.Context, []SpeedNode) ([]PingProbeResult, error)
	probeTargets func(context.Context, []PingTarget) ([]PingProbeResult, error)
}

// NewPingWorkload creates a network ping benchmark.
func NewPingWorkload() bench.Workload {
	return &pingWorkload{targets: DefaultPingTargets("v4"), targetsSet: true}

}

// NewPingWorkloadWithManifest creates a ping workload pinned to a selected
// manifest and IP-family view instead of reading the embedded catalog.
func NewPingWorkloadWithManifest(manifest nodecatalog.Manifest, ipFamily string) bench.Workload {
	return &pingWorkload{targets: PingTargetsFromManifest(manifest, ipFamily), targetsSet: true}
}

func (w *pingWorkload) Name() string        { return "Net Ping" }
func (w *pingWorkload) Category() string    { return bench.CategoryNetwork }
func (w *pingWorkload) Description() string { return "TCP latency / jitter / packet loss to all nodes" }
func (w *pingWorkload) Validate() error     { return nil }
func (w *pingWorkload) SkipWarmup() bool    { return true }
func (w *pingWorkload) MaxIterations() int  { return 1 }

func (w *pingWorkload) Throughput(int64, time.Duration) (float64, string) {
	return w.avgMs, "ms avg"
}

func (w *pingWorkload) Detail() string { return w.detail }

func (w *pingWorkload) Run(ctx context.Context) (time.Duration, int64, error) {
	if w.detail != "" {
		return w.elapsed, w.okCount, w.runErr // cached
	}

	start := time.Now()
	var probes []PingProbeResult
	var probeErr error
	if w.targetsSet {
		probeTargets := w.probeTargets
		if probeTargets == nil {
			probeTargets = ProbePingTargets
		}
		probes, probeErr = probeTargets(ctx, w.targets)
	} else {
		nodes := DefaultNodes()
		probeNodes := w.probeNodes
		if probeNodes == nil {
			probeNodes = ProbePingNodes
		}
		probes, probeErr = probeNodes(ctx, nodes)
	}
	elapsed := time.Since(start)

	var allRTTs []float64
	var parts []string
	for _, r := range probes {
		if r.Status != "ok" {
			parts = append(parts, fmt.Sprintf("%s=FAIL", r.Name))
			continue
		}
		allRTTs = append(allRTTs, r.AvgLatencyMs)
		parts = append(parts, fmt.Sprintf("%s=%.1fms/%.1fmsj/%.0f%%loss", r.Name, r.AvgLatencyMs, r.JitterMs, r.PacketLoss))
	}

	w.avgMs = 0
	w.lossPct = 0
	if len(allRTTs) > 0 {
		w.avgMs = avgFloat(allRTTs)
	}
	totalProbes := 0
	totalLost := 0
	for _, r := range probes {
		totalProbes += r.Sent
		totalLost += r.Sent - r.Received
	}
	if totalProbes > 0 {
		w.lossPct = float64(totalLost) / float64(totalProbes) * 100
	}

	w.elapsed = elapsed
	w.okCount = int64(len(allRTTs))
	w.detail = fmt.Sprintf("avg=%.1fms jitter=%.1fms loss=%.1f%% | %s", w.avgMs, globalJitter(allRTTs), w.lossPct, strings.Join(parts, " "))
	w.runErr = probeErr
	if w.okCount == 0 && w.runErr == nil {
		w.runErr = fmt.Errorf("all %d ping targets failed", len(probes))
	}
	return w.elapsed, w.okCount, w.runErr
}

func pingNode(ctx context.Context, node SpeedNode) nodePingResult {
	host := hostFromURL(node.TestURL)
	if host == "" {
		return nodePingResult{name: node.Name, region: node.Region, host: host, err: fmt.Errorf("invalid ping target URL")}
	}

	evidence := probeTCP(ctx, "tcp", net.JoinHostPort(host, fmt.Sprintf("%d", pingPort)), defaultPingDial)
	loss := float64(pingProbes-len(evidence.rtts)) / float64(pingProbes) * 100
	return nodePingResult{
		name:            node.Name,
		region:          node.Region,
		host:            host,
		rtts:            evidence.rtts,
		loss:            loss,
		connectionState: evidence.connectionState(),
		message:         evidence.responseMessage(),
		err:             evidence.lastErr,
	}
}

func pingTarget(ctx context.Context, target PingTarget) PingProbeResult {
	return pingTargetWithDial(ctx, target, defaultPingDial)
}

func pingTargetWithDial(ctx context.Context, target PingTarget, dial pingDialFunc) PingProbeResult {
	port := target.Port
	if port <= 0 {
		port = pingPort
	}
	id := strings.TrimSpace(target.ID)
	if id == "" {
		id = strings.ToLower(strings.ReplaceAll(target.Name, " ", "-"))
	}
	result := PingProbeResult{
		ID:            id,
		Name:          target.Name,
		Region:        target.Region,
		City:          target.City,
		Carrier:       target.Carrier,
		ASN:           target.ASN,
		IPFamily:      target.IPFamily,
		Protocol:      target.Protocol,
		Source:        target.Source,
		ProbeProtocol: "tcp-connect",
		ProbeTool:     "go-net-dialer",
		Target:        target.Endpoint,
		Port:          port,
		Sent:          pingProbes,
	}

	network := "tcp"
	switch strings.ToLower(strings.TrimSpace(target.IPFamily)) {
	case "v4", "ipv4":
		network = "tcp4"
	case "v6", "ipv6":
		network = "tcp6"
	}

	evidence := probeTCP(ctx, network, net.JoinHostPort(target.Endpoint, fmt.Sprintf("%d", port)), dial)
	result.ConnectionState = evidence.connectionState()
	result.Received = len(evidence.rtts)
	result.PacketLoss = float64(pingProbes-len(evidence.rtts)) / float64(pingProbes) * 100
	if len(evidence.rtts) == 0 {
		result.Status = "error"
		if evidence.lastErr != nil {
			result.Message = evidence.lastErr.Error()
		} else {
			result.Message = "no successful probes"
		}
		return result
	}
	result.Status = "ok"
	result.Message = evidence.responseMessage()
	result.AvgLatencyMs = avgDuration(evidence.rtts).Seconds() * 1000
	result.JitterMs = jitterDuration(evidence.rtts).Seconds() * 1000
	return result
}

func defaultPingDial(ctx context.Context, network, address string) (net.Conn, error) {
	dialer := &net.Dialer{Timeout: pingTimeout}
	return dialer.DialContext(ctx, network, address)
}

func probeTCP(ctx context.Context, network, address string, dial pingDialFunc) tcpPingEvidence {
	evidence := tcpPingEvidence{rtts: make([]time.Duration, 0, pingProbes)}
	for i := 0; i < pingProbes; i++ {
		probeCtx, cancel := context.WithTimeout(ctx, pingTimeout)
		start := time.Now()
		conn, err := dial(probeCtx, network, address)
		elapsed := time.Since(start)
		cancel()
		if err != nil {
			evidence.lastErr = err
			if isTCPRST(err) {
				evidence.rstResponses++
				evidence.lastRSTErr = err
				evidence.rtts = append(evidence.rtts, elapsed)
			}
			continue
		}
		if conn != nil {
			_ = conn.Close()
		}
		evidence.rtts = append(evidence.rtts, elapsed)
	}
	return evidence
}

func (e tcpPingEvidence) connectionState() string {
	open := len(e.rtts) - e.rstResponses
	switch {
	case open > 0 && e.rstResponses > 0:
		return PingConnectionStateMixed
	case open > 0:
		return PingConnectionStateOpen
	case e.rstResponses > 0:
		return PingConnectionStateRefused
	default:
		return PingConnectionStateNoResponse
	}
}

func (e tcpPingEvidence) responseMessage() string {
	if e.lastRSTErr == nil {
		return ""
	}
	return "target responded with TCP RST: " + e.lastRSTErr.Error()
}

func isTCPRST(err error) bool {
	// Windows reports WSAECONNRESET/WSAECONNREFUSED as errno 10054/10061
	// rather than the platform-independent syscall values.
	return errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, syscall.ECONNREFUSED) ||
		errors.Is(err, syscall.Errno(10054)) ||
		errors.Is(err, syscall.Errno(10061))
}

// --- helpers ---

func hostFromURL(rawURL string) string {
	s := rawURL
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	if i := strings.Index(s, "/"); i >= 0 {
		s = s[:i]
	}
	host, _, _ := net.SplitHostPort(s)
	if host == "" {
		return s
	}
	return host
}

func avgDuration(ds []time.Duration) time.Duration {
	if len(ds) == 0 {
		return 0
	}
	var sum time.Duration
	for _, d := range ds {
		sum += d
	}
	return sum / time.Duration(len(ds))
}

func jitterDuration(ds []time.Duration) time.Duration {
	if len(ds) < 2 {
		return 0
	}
	avg := avgDuration(ds)
	var sum float64
	for _, d := range ds {
		diff := d - avg
		sum += math.Abs(float64(diff))
	}
	return time.Duration(sum / float64(len(ds)))
}

func avgFloat(fs []float64) float64 {
	if len(fs) == 0 {
		return 0
	}
	var sum float64
	for _, f := range fs {
		sum += f
	}
	return sum / float64(len(fs))
}

func globalJitter(nodeAvgs []float64) float64 {
	if len(nodeAvgs) < 2 {
		return 0
	}
	sort.Float64s(nodeAvgs)
	q1 := nodeAvgs[len(nodeAvgs)/4]
	q3 := nodeAvgs[3*len(nodeAvgs)/4]
	return q3 - q1
}
