package netio

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os/exec"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cloudapp3/vmbench/bench"
	"github.com/cloudapp3/vmbench/nodecatalog"
)

const (
	maxTTL                 = 30
	traceTimeout           = 45 * time.Second
	tracePort              = 80
	maxConcurrentTraces    = 4
	maxTraceErrorOutputLen = 256
)

var (
	traceHopLineRE = regexp.MustCompile(`^\s*(\d+)(?:\?)?:?\s+(.*)$`)
	traceRTTRE     = regexp.MustCompile(`(?i)(<)?\s*(\d+(?:\.\d+)?)\s*ms`)
)

// TraceTarget defines a traceroute destination (following vmpulse's nxtrace.org approach).
type TraceTarget struct {
	ID       string `json:"id,omitempty"`
	Name     string `json:"name"`
	Region   string `json:"region,omitempty"`
	City     string `json:"city"`
	Carrier  string `json:"carrier"`
	AS       int    `json:"as"`
	IPFamily string `json:"ip_family,omitempty"`
	Protocol string `json:"protocol,omitempty"`
	Endpoint string `json:"endpoint"`
	Port     int    `json:"port,omitempty"`
	Source   string `json:"source,omitempty"`
}

// DefaultTraceTargets returns the embedded catalog's IPv4 route targets.
func DefaultTraceTargets() []TraceTarget {
	manifest, err := nodecatalog.Embedded()
	if err != nil {
		return nil
	}
	return TraceTargetsFromManifest(manifest, "v4")
}

// Hop represents one traceroute hop.
type Hop struct {
	TTL     int     `json:"ttl"`
	IP      string  `json:"ip,omitempty"`
	ASN     string  `json:"asn,omitempty"`
	RTTMs   float64 `json:"rtt_ms,omitempty"`
	Timeout bool    `json:"timeout,omitempty"`
}

// TraceProbeResult stores one traceroute outcome.
type TraceProbeResult struct {
	Target             TraceTarget          `json:"target"`
	ResolvedTarget     string               `json:"resolved_target,omitempty"`
	DestinationReached *bool                `json:"destination_reached,omitempty"`
	Status             string               `json:"status,omitempty"`
	ProbeProtocol      string               `json:"probe_protocol"`
	ProbeTool          string               `json:"probe_tool"`
	Hops               []Hop                `json:"hops,omitempty"`
	Classification     *RouteClassification `json:"classification,omitempty"`
	ObservedASNs       []string             `json:"observed_asns,omitempty"`
	Error              string               `json:"error,omitempty"`
}

const (
	TraceStatusOK      = "ok"
	TraceStatusPartial = "partial"
	TraceStatusError   = "error"
)

// EffectiveStatus normalizes new reachability evidence while retaining the
// pre-status interpretation for older JSON reports.
func (r TraceProbeResult) EffectiveStatus() string {
	if strings.TrimSpace(r.Error) != "" {
		return TraceStatusError
	}
	status := strings.ToLower(strings.TrimSpace(r.Status))
	switch status {
	case TraceStatusPartial, TraceStatusError:
		return status
	case TraceStatusOK:
		if r.DestinationReached != nil && !*r.DestinationReached {
			if len(r.Hops) > 0 {
				return TraceStatusPartial
			}
			return TraceStatusError
		}
		return TraceStatusOK
	}
	if r.DestinationReached != nil {
		if *r.DestinationReached {
			return TraceStatusOK
		}
		if len(r.Hops) > 0 {
			return TraceStatusPartial
		}
		return TraceStatusError
	}
	// Reports produced before reachability evidence was added treated every
	// error-free trace as successful.
	return TraceStatusOK
}

type traceProbeEvidence struct {
	resolvedTarget     string
	destinationReached bool
	hops               []Hop
	protocol           string
	tool               string
	err                error
}

// ProbeTracerouteTargets runs system traceroute commands against the provided targets.
func ProbeTracerouteTargets(ctx context.Context, targets []TraceTarget) ([]TraceProbeResult, error) {
	return probeTracerouteTargetEvidence(ctx, targets, systemTracerouteTargetEvidence)
}

func probeTracerouteTargets(ctx context.Context, targets []TraceTarget, probe func(context.Context, string) ([]Hop, error)) ([]TraceProbeResult, error) {
	return probeTracerouteTargetSpecs(ctx, targets, func(ctx context.Context, target TraceTarget) ([]Hop, error) {
		return probe(ctx, target.Endpoint)
	})
}

func probeTracerouteTargetSpecs(ctx context.Context, targets []TraceTarget, probe func(context.Context, TraceTarget) ([]Hop, error)) ([]TraceProbeResult, error) {
	return probeTracerouteTargetEvidence(ctx, targets, func(ctx context.Context, target TraceTarget) traceProbeEvidence {
		hops, err := probe(ctx, target)
		resolvedTarget := normalizedTraceIP(target.Endpoint)
		return traceProbeEvidence{
			resolvedTarget:     resolvedTarget,
			destinationReached: traceDestinationReached(hops, resolvedTarget),
			hops:               hops,
			protocol:           "injected",
			tool:               "injected",
			err:                err,
		}
	})
}

func probeTracerouteTargetEvidence(ctx context.Context, targets []TraceTarget, probe func(context.Context, TraceTarget) traceProbeEvidence) ([]TraceProbeResult, error) {
	results := make([]TraceProbeResult, len(targets))
	if len(targets) == 0 {
		return results, nil
	}

	workers := maxConcurrentTraces
	if workers > len(targets) {
		workers = len(targets)
	}
	jobs := make(chan int, len(targets))
	for i := range targets {
		jobs <- i
	}
	close(jobs)

	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range jobs {
				target := targets[idx]
				evidence := probe(ctx, target)
				reached := evidence.destinationReached
				result := TraceProbeResult{
					Target:             target,
					ResolvedTarget:     strings.TrimSpace(evidence.resolvedTarget),
					DestinationReached: &reached,
					ProbeProtocol:      firstNonEmpty(strings.TrimSpace(evidence.protocol), "unknown"),
					ProbeTool:          firstNonEmpty(strings.TrimSpace(evidence.tool), "unknown"),
					Hops:               evidence.hops,
				}
				if evidence.err != nil {
					result.Status = TraceStatusError
					result.Error = evidence.err.Error()
					results[idx] = result
					continue
				}
				if reached {
					result.Status = TraceStatusOK
				} else if len(evidence.hops) > 0 {
					result.Status = TraceStatusPartial
				} else {
					result.Status = TraceStatusError
					result.Error = "traceroute produced no valid hops"
				}
				results[idx] = result
			}
		}()
	}
	wg.Wait()
	return results, nil
}

// tracerouteWorkload performs system traceroute to Chinese carrier endpoints.
type tracerouteWorkload struct {
	detail     string
	elapsed    time.Duration
	hopSum     int
	runErr     error
	targets    []TraceTarget
	targetsSet bool
}

// NewTracerouteWorkload creates a route tracing benchmark.
func NewTracerouteWorkload() bench.Workload {
	return &tracerouteWorkload{targets: DefaultTraceTargets(), targetsSet: true}
}

// NewTracerouteWorkloadWithManifest creates a route workload pinned to a
// selected manifest and IP-family view instead of reading the embedded catalog.
func NewTracerouteWorkloadWithManifest(manifest nodecatalog.Manifest, ipFamily string) bench.Workload {
	return &tracerouteWorkload{targets: TraceTargetsFromManifest(manifest, ipFamily), targetsSet: true}
}

func (w *tracerouteWorkload) Name() string     { return "Net Traceroute" }
func (w *tracerouteWorkload) Category() string { return bench.CategoryNetwork }
func (w *tracerouteWorkload) Description() string {
	return "System traceroute to versioned China carrier, CERNET, and CSTNET targets"
}
func (w *tracerouteWorkload) Validate() error  { return nil }
func (w *tracerouteWorkload) SkipWarmup() bool { return true }
func (w *tracerouteWorkload) MaxIterations() int {
	return 1
}

func (w *tracerouteWorkload) Throughput(int64, time.Duration) (float64, string) {
	return float64(w.hopSum), "hops"
}

func (w *tracerouteWorkload) Detail() string { return w.detail }

func (w *tracerouteWorkload) Run(ctx context.Context) (time.Duration, int64, error) {
	if w.detail != "" {
		return w.elapsed, int64(w.hopSum), w.runErr
	}
	start := time.Now()
	targets := w.targets
	if !w.targetsSet {
		targets = DefaultTraceTargets()
	}
	results, err := ProbeTracerouteTargets(ctx, targets)
	w.elapsed = time.Since(start)
	if err != nil {
		return 0, 0, err
	}
	var parts []string
	totalHops := 0
	okCount := 0
	firstError := ""
	for _, r := range results {
		status := r.EffectiveStatus()
		if status == TraceStatusError {
			parts = append(parts, fmt.Sprintf("%s: ERR", r.Target.Name))
			if firstError == "" {
				firstError = r.Error
			}
			continue
		}
		if status == TraceStatusOK {
			okCount++
		}
		n := len(r.Hops)
		totalHops += n
		var lastHop string
		for i := n - 1; i >= 0; i-- {
			if r.Hops[i].IP != "" && !r.Hops[i].Timeout {
				lastHop = r.Hops[i].IP
				break
			}
		}
		parts = append(parts, fmt.Sprintf("%s: %s %dhops last=%s", r.Target.Name, strings.ToUpper(status), n, lastHop))
	}
	w.hopSum = totalHops
	w.detail = strings.Join(parts, "\n")
	if okCount == 0 {
		if firstError != "" {
			w.runErr = fmt.Errorf("none of %d traceroutes reached the destination: %s", len(results), firstError)
		} else {
			w.runErr = fmt.Errorf("none of %d traceroutes reached the destination", len(results))
		}
		return w.elapsed, 0, w.runErr
	}
	return w.elapsed, int64(totalHops), nil
}

type traceCommandSpec struct {
	name     string
	protocol string
	args     []string
}

type traceLookPathFunc func(string) (string, error)
type traceRunCommandFunc func(context.Context, string, ...string) ([]byte, error)

func systemTraceroute(ctx context.Context, host string) ([]Hop, error) {
	return systemTracerouteWith(ctx, host, exec.LookPath, runTraceCommand)
}

func systemTracerouteTarget(ctx context.Context, target TraceTarget) ([]Hop, error) {
	evidence := systemTracerouteTargetEvidence(ctx, target)
	return evidence.hops, evidence.err
}

func systemTracerouteTargetEvidence(ctx context.Context, target TraceTarget) traceProbeEvidence {
	port := target.Port
	if port <= 0 {
		port = tracePort
	}
	return systemTracerouteWithEvidence(ctx, target.Endpoint, target.IPFamily, port, exec.LookPath, runTraceCommand)
}

func systemTracerouteWith(ctx context.Context, host string, lookPath traceLookPathFunc, run traceRunCommandFunc) ([]Hop, error) {
	return systemTracerouteWithPort(ctx, host, tracePort, lookPath, run)
}

func systemTracerouteWithPort(ctx context.Context, host string, port int, lookPath traceLookPathFunc, run traceRunCommandFunc) ([]Hop, error) {
	evidence := systemTracerouteWithEvidence(ctx, host, "", port, lookPath, run)
	return evidence.hops, evidence.err
}

func systemTracerouteWithEvidence(ctx context.Context, host, family string, port int, lookPath traceLookPathFunc, run traceRunCommandFunc) traceProbeEvidence {
	targetIP, err := resolveTraceTargetForFamily(ctx, host, family)
	if err != nil {
		return traceProbeEvidence{protocol: "none", tool: "resolver", err: err}
	}
	resolvedFamily := traceIPFamily(targetIP)
	specs := traceCommandSpecsForTarget(targetIP, port, resolvedFamily)

	found := false
	lastProtocol := "none"
	lastTool := "none"
	errors := make([]string, 0, 4)
	traceCtx, cancel := context.WithTimeout(ctx, traceTimeout)
	defer cancel()
	for _, spec := range specs {
		path, pathErr := lookPath(spec.name)
		if pathErr != nil {
			continue
		}
		found = true
		lastProtocol = spec.protocol
		lastTool = spec.name
		output, commandErr := run(traceCtx, path, spec.args...)

		hops, parseErr := parseTracerouteOutput(output)
		if parseErr == nil {
			return traceProbeEvidence{
				resolvedTarget:     targetIP,
				destinationReached: traceDestinationReached(hops, targetIP),
				hops:               hops,
				protocol:           spec.protocol,
				tool:               spec.name,
			}
		}
		if commandErr != nil {
			errors = append(errors, fmt.Sprintf("%s: %v: %s", spec.name, commandErr, truncateTraceOutput(output)))
			continue
		}
		errors = append(errors, fmt.Sprintf("%s: %v", spec.name, parseErr))
	}
	if !found {
		return traceProbeEvidence{
			resolvedTarget: targetIP,
			protocol:       "none",
			tool:           "none",
			err:            fmt.Errorf("no traceroute command available (tried %s)", strings.Join(traceCommandSpecNames(specs), ", ")),
		}
	}
	return traceProbeEvidence{
		resolvedTarget: targetIP,
		protocol:       lastProtocol,
		tool:           lastTool,
		err:            fmt.Errorf("traceroute %s produced no valid hops: %s", host, strings.Join(errors, "; ")),
	}
}

func resolveTraceTarget(ctx context.Context, host string) (string, error) {
	return resolveTraceTargetForFamily(ctx, host, "")
}

func resolveTraceTargetForFamily(ctx context.Context, host, family string) (string, error) {
	return resolveTraceTargetWith(ctx, host, family, net.DefaultResolver.LookupIPAddr)
}

func resolveTraceTargetWith(ctx context.Context, host, family string, lookup func(context.Context, string) ([]net.IPAddr, error)) (string, error) {
	host = strings.TrimSpace(host)
	if ip := net.ParseIP(host); ip != nil {
		if !traceIPMatchesFamily(ip, family) {
			return "", fmt.Errorf("resolve %s: address does not match requested %s family", host, strings.TrimSpace(family))
		}
		return ip.String(), nil
	}
	ips, err := lookup(ctx, host)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", host, err)
	}
	requested := normalizeTraceFamily(family)
	if requested == "v6" {
		for _, addr := range ips {
			if addr.IP != nil && addr.IP.To4() == nil {
				return addr.IP.String(), nil
			}
		}
		return "", fmt.Errorf("no IPv6 address for %s", host)
	}
	for _, addr := range ips {
		if addr.IP.To4() != nil {
			return addr.IP.String(), nil
		}
	}
	if requested == "v4" {
		return "", fmt.Errorf("no IPv4 address for %s", host)
	}
	for _, addr := range ips {
		if addr.IP != nil {
			return addr.IP.String(), nil
		}
	}
	return "", fmt.Errorf("no IP for %s", host)
}

func traceCommandSpecs(target string) []traceCommandSpec {
	return traceCommandSpecsWithPort(target, tracePort)
}

func traceCommandSpecsWithPort(target string, port int) []traceCommandSpec {
	return traceCommandSpecsForTarget(target, port, traceIPFamily(target))
}

func traceCommandSpecsForTarget(target string, port int, family string) []traceCommandSpec {
	if port <= 0 || port > 65535 {
		port = tracePort
	}
	family = normalizeTraceFamily(family)
	familyFlag := "-4"
	if family == "v6" {
		familyFlag = "-6"
	}
	if runtime.GOOS == "windows" {
		return []traceCommandSpec{{name: "tracert", protocol: "icmp", args: []string{familyFlag, "-d", "-h", strconv.Itoa(maxTTL), "-w", "3000", target}}}
	}

	tracerouteName := "traceroute"
	tracerouteArgs := []string{"-n", "-p", strconv.Itoa(port), "-q", "1", "-m", strconv.Itoa(maxTTL), "-w", "1", target}
	tracepathArgs := []string{"-n", "-p", strconv.Itoa(port), "-m", strconv.Itoa(maxTTL), target}
	if runtime.GOOS == "linux" {
		tracerouteArgs = []string{familyFlag, "-n", "-T", "-p", strconv.Itoa(port), "-q", "1", "-m", strconv.Itoa(maxTTL), "-w", "1", target}
		tracepathArgs = append([]string{familyFlag}, tracepathArgs...)
	} else if runtime.GOOS == "darwin" && family == "v6" {
		tracerouteName = "traceroute6"
	}
	tracerouteProtocol := "udp"
	if runtime.GOOS == "linux" {
		tracerouteProtocol = "tcp"
	}
	return []traceCommandSpec{
		{name: tracerouteName, protocol: tracerouteProtocol, args: tracerouteArgs},
		{name: "tcptraceroute", protocol: "tcp", args: []string{"-n", "-q", "1", "-m", strconv.Itoa(maxTTL), "-w", "1", target, strconv.Itoa(port)}},
		{name: "tracepath", protocol: "udp", args: tracepathArgs},
	}
}

func traceCommandNames() []string {
	return traceCommandSpecNames(traceCommandSpecs("target"))
}

func traceCommandSpecNames(specs []traceCommandSpec) []string {
	names := make([]string, 0, len(specs))
	for _, spec := range specs {
		names = append(names, spec.name)
	}
	return names
}

func normalizeTraceFamily(family string) string {
	switch strings.ToLower(strings.TrimSpace(family)) {
	case "v6", "ipv6", "6":
		return "v6"
	case "v4", "ipv4", "4":
		return "v4"
	default:
		return ""
	}
}

func traceIPFamily(value string) string {
	ip := net.ParseIP(strings.TrimSpace(value))
	if ip == nil {
		return ""
	}
	if ip.To4() != nil {
		return "v4"
	}
	return "v6"
}

func traceIPMatchesFamily(ip net.IP, family string) bool {
	switch normalizeTraceFamily(family) {
	case "v4":
		return ip.To4() != nil
	case "v6":
		return ip.To4() == nil
	default:
		return true
	}
}

func normalizedTraceIP(value string) string {
	ip := net.ParseIP(strings.TrimSpace(value))
	if ip == nil {
		return ""
	}
	return ip.String()
}

func traceDestinationReached(hops []Hop, target string) bool {
	targetIP := net.ParseIP(strings.TrimSpace(target))
	if targetIP == nil {
		return false
	}
	for _, hop := range hops {
		if hop.Timeout {
			continue
		}
		hopIP := net.ParseIP(strings.TrimSpace(hop.IP))
		if hopIP != nil && hopIP.Equal(targetIP) {
			return true
		}
	}
	return false
}

func runTraceCommand(ctx context.Context, path string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, path, args...).CombinedOutput()
}

func parseTracerouteOutput(output []byte) ([]Hop, error) {
	hopsByTTL := make(map[int]Hop)
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		match := traceHopLineRE.FindStringSubmatch(scanner.Text())
		if len(match) != 3 {
			continue
		}
		ttl, err := strconv.Atoi(match[1])
		if err != nil || ttl < 1 || ttl > maxTTL {
			continue
		}
		rest := match[2]
		hop := Hop{TTL: ttl, IP: traceLineIP(rest), RTTMs: traceLineRTT(rest)}
		hop.Timeout = hop.IP == ""
		previous, exists := hopsByTTL[ttl]
		if !exists || (previous.Timeout && !hop.Timeout) {
			hopsByTTL[ttl] = hop
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("parse traceroute output: %w", err)
	}

	ttls := make([]int, 0, len(hopsByTTL))
	validHops := 0
	for ttl, hop := range hopsByTTL {
		ttls = append(ttls, ttl)
		if !hop.Timeout && hop.IP != "" {
			validHops++
		}
	}
	if validHops == 0 {
		return nil, fmt.Errorf("traceroute output contains no valid hops")
	}
	sort.Ints(ttls)
	hops := make([]Hop, 0, len(ttls))
	for _, ttl := range ttls {
		hops = append(hops, hopsByTTL[ttl])
	}
	return hops, nil
}

func traceLineIP(line string) string {
	for _, field := range strings.Fields(line) {
		candidate := strings.Trim(field, "()[]<>,")
		if idx := strings.IndexByte(candidate, '!'); idx >= 0 {
			candidate = candidate[:idx]
		}
		if zone := strings.LastIndexByte(candidate, '%'); zone >= 0 {
			candidate = candidate[:zone]
		}
		if ip := net.ParseIP(candidate); ip != nil {
			return ip.String()
		}
	}
	return ""
}

func traceLineRTT(line string) float64 {
	match := traceRTTRE.FindStringSubmatch(line)
	if len(match) != 3 {
		return 0
	}
	value, err := strconv.ParseFloat(match[2], 64)
	if err != nil {
		return 0
	}
	if match[1] == "<" {
		return value / 2
	}
	return value
}

func truncateTraceOutput(output []byte) string {
	text := strings.Join(strings.Fields(string(output)), " ")
	if len(text) > maxTraceErrorOutputLen {
		return text[:maxTraceErrorOutputLen] + "..."
	}
	if text == "" {
		return "no output"
	}
	return text
}
