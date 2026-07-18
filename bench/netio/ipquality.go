package netio

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/cloudapp3/vmbench/bench"
)

// IPBasicInfo stores coarse public IP metadata.
type IPBasicInfo struct {
	Source      string `json:"source,omitempty"`
	IP          string `json:"ip,omitempty"`
	Country     string `json:"country,omitempty"`
	CountryCode string `json:"country_code,omitempty"`
	ASN         int64  `json:"asn,omitempty"`
	Org         string `json:"org,omitempty"`
	ISP         string `json:"isp,omitempty"`
	Hosting     bool   `json:"hosting,omitempty"`
	Proxy       bool   `json:"proxy,omitempty"`
	Error       string `json:"error,omitempty"`
}

// IPRiskSummary stores risk heuristics and DNSBL findings.
type IPRiskSummary struct {
	RiskLevel        string   `json:"risk_level,omitempty"`
	Summary          string   `json:"summary,omitempty"`
	DNSBLSupported   bool     `json:"dnsbl_supported"`
	DNSBLTool        string   `json:"dnsbl_tool,omitempty"`
	DNSBLMessage     string   `json:"dnsbl_message,omitempty"`
	DNSBLListedCount int      `json:"dnsbl_listed_count,omitempty"`
	DNSBLListed      []string `json:"dnsbl_listed,omitempty"`
}

// PortProbe stores one outbound email port probe.
type PortProbe struct {
	Port      int     `json:"port,omitempty"`
	Title     string  `json:"title,omitempty"`
	Supported bool    `json:"supported"`
	Status    string  `json:"status,omitempty"`
	Target    string  `json:"target,omitempty"`
	Method    string  `json:"method,omitempty"`
	LatencyMs float64 `json:"latency_ms,omitempty"`
	Message   string  `json:"message,omitempty"`
}

// IPScore stores a normalized summary score.
type IPScore struct {
	Total    int    `json:"total"`
	MaxTotal int    `json:"max_total"`
	Level    string `json:"level"`
}

// IPQualityResult stores structured IP quality assessment output.
type IPQualityResult struct {
	BasicInfo   *IPBasicInfo   `json:"basic_info,omitempty"`
	RiskSummary *IPRiskSummary `json:"risk_summary,omitempty"`
	Port25      *PortProbe     `json:"port_25,omitempty"`
	MailPorts   []PortProbe    `json:"mail_ports,omitempty"`
	Score       *IPScore       `json:"score,omitempty"`
}

type dnsblCheckResult struct {
	Listed  []string
	Checked int
	Errors  []string
}

type ipQualityDependencies struct {
	queryInfo  func(context.Context) (*IPBasicInfo, riskFlags, error)
	publicIP   func(context.Context) (string, error)
	checkDNSBL func(context.Context, string) dnsblCheckResult
	mailPorts  func(context.Context, []int) []PortProbe
}

// ProbeIPQuality performs the structured IP quality assessment.
func ProbeIPQuality(ctx context.Context) (*IPQualityResult, error) {
	return probeIPQuality(ctx, ipQualityDependencies{
		queryInfo:  queryIPAPIInfo,
		publicIP:   getPublicIP,
		checkDNSBL: checkDNSBL,
		mailPorts:  ProbeMailPorts,
	})
}

func probeIPQuality(ctx context.Context, deps ipQualityDependencies) (*IPQualityResult, error) {
	result := &IPQualityResult{}
	info, flags, err := deps.queryInfo(ctx)
	if err != nil {
		result.BasicInfo = &IPBasicInfo{Source: "ip-api.com", Error: err.Error()}
		return result, fmt.Errorf("ip quality: IP metadata unavailable: %w", err)
	}
	if info == nil {
		return result, fmt.Errorf("ip quality: IP metadata unavailable: empty response")
	}
	result.BasicInfo = info

	publicIP := strings.TrimSpace(info.IP)
	if parsed := net.ParseIP(publicIP); parsed == nil || parsed.To4() == nil {
		publicIP, err = deps.publicIP(ctx)
		if err != nil {
			return result, fmt.Errorf("ip quality: public IPv4 unavailable: %w", err)
		}
		publicIP = strings.TrimSpace(publicIP)
		parsed := net.ParseIP(publicIP)
		if parsed == nil || parsed.To4() == nil {
			return result, fmt.Errorf("ip quality: public IPv4 unavailable: invalid address %q", publicIP)
		}
		info.IP = parsed.String()
	}

	dnsbl := deps.checkDNSBL(ctx, publicIP)
	dnsblMessage := ""
	if len(dnsbl.Errors) > 0 {
		dnsblMessage = strings.Join(dnsbl.Errors, "; ")
	}
	if dnsbl.Checked != len(dnsblZones) {
		if dnsblMessage == "" {
			dnsblMessage = fmt.Sprintf("only %d/%d DNSBL zones returned a conclusive result", dnsbl.Checked, len(dnsblZones))
		}
		result.RiskSummary = &IPRiskSummary{
			DNSBLSupported: false,
			DNSBLTool:      "dnsbl",
			DNSBLMessage:   dnsblMessage,
		}
		return result, fmt.Errorf("ip quality: DNSBL unavailable (%d/%d zones checked): %s", dnsbl.Checked, len(dnsblZones), dnsblMessage)
	}

	score := 100
	if flags.proxy {
		score -= 18
	}
	if flags.hosting {
		score -= 6
	}

	listed := dnsbl.Listed
	if len(listed) > 0 {
		deduct := len(listed) * 12
		if deduct > 30 {
			deduct = 30
		}
		score -= deduct
	}

	mailPorts := deps.mailPorts(ctx, []int{25})
	var port25 *PortProbe
	found25Open := false
	for idx := range mailPorts {
		if mailPorts[idx].Port == 25 {
			cp := mailPorts[idx]
			port25 = &cp
			if cp.Status == "open" {
				found25Open = true
			}
		}
	}
	if !found25Open {
		score -= 5
	}
	if score < 0 {
		score = 0
	}

	level := scoreLevel(score)
	summary := []string{fmt.Sprintf("score %d/100", score)}
	if flags.proxy {
		summary = append(summary, "proxy detected")
	}
	if flags.hosting {
		summary = append(summary, "hosting ASN")
	}
	if len(listed) > 0 {
		summary = append(summary, fmt.Sprintf("dnsbl listed x%d", len(listed)))
	}
	if !found25Open {
		summary = append(summary, "port25 blocked")
	}

	result.RiskSummary = &IPRiskSummary{
		RiskLevel:        strings.ToLower(level),
		Summary:          strings.Join(summary, " · "),
		DNSBLSupported:   true,
		DNSBLTool:        "dnsbl",
		DNSBLMessage:     dnsblMessage,
		DNSBLListedCount: len(listed),
		DNSBLListed:      listed,
	}
	result.Port25 = port25
	result.MailPorts = mailPorts
	result.Score = &IPScore{Total: score, MaxTotal: 100, Level: level}
	return result, nil
}

// ipQualityWorkload checks IP reputation, DNSBL listings, and email port availability.
type ipQualityWorkload struct {
	detail  string
	score   int // 0-100
	elapsed time.Duration
}

// NewIPQualityWorkload creates an IP quality assessment benchmark.
func NewIPQualityWorkload() bench.Workload {
	return &ipQualityWorkload{}
}

func (w *ipQualityWorkload) Name() string     { return "Net IP Quality" }
func (w *ipQualityWorkload) Category() string { return bench.CategoryNetwork }
func (w *ipQualityWorkload) Description() string {
	return "IP reputation / DNSBL / mail port detection"
}
func (w *ipQualityWorkload) Validate() error  { return nil }
func (w *ipQualityWorkload) SkipWarmup() bool { return true }
func (w *ipQualityWorkload) MaxIterations() int {
	return 1
}

func (w *ipQualityWorkload) Throughput(int64, time.Duration) (float64, string) {
	return float64(w.score), "/100"
}

func (w *ipQualityWorkload) Detail() string { return w.detail }

func (w *ipQualityWorkload) Run(ctx context.Context) (time.Duration, int64, error) {
	if w.detail != "" {
		return w.elapsed, int64(w.score), nil
	}
	start := time.Now()
	result, err := ProbeIPQuality(ctx)
	w.elapsed = time.Since(start)
	if err != nil {
		return 0, 0, err
	}
	if result.Score != nil {
		w.score = result.Score.Total
	}
	parts := make([]string, 0, 4)
	if result.BasicInfo != nil {
		parts = append(parts, fmt.Sprintf("IP:%s %s %s", result.BasicInfo.IP, result.BasicInfo.CountryCode, firstNonEmpty(result.BasicInfo.ISP, result.BasicInfo.Org)))
	}
	if result.RiskSummary != nil {
		parts = append(parts, result.RiskSummary.Summary)
	}
	if len(result.MailPorts) > 0 {
		portParts := make([]string, 0, len(result.MailPorts))
		for _, p := range result.MailPorts {
			portParts = append(portParts, fmt.Sprintf("%d:%s", p.Port, strings.ToUpper(firstNonEmpty(p.Status, "unknown"))))
		}
		parts = append(parts, "Ports:"+strings.Join(portParts, ","))
	}
	w.detail = strings.Join(parts, " | ")
	return w.elapsed, int64(w.score), nil
}

// --- IP API ---

type ipAPIResponse struct {
	Status      string `json:"status"`
	Message     string `json:"message"`
	IP          string `json:"query"`
	Country     string `json:"country"`
	CountryCode string `json:"countryCode"`
	ISP         string `json:"isp"`
	Org         string `json:"org"`
	AS          string `json:"as"`
	Hosting     bool   `json:"hosting"`
	Proxy       bool   `json:"proxy"`
}

type riskFlags struct {
	proxy   bool
	hosting bool
}

var asnRe = regexp.MustCompile(`AS(\d+)`)

func queryIPAPIInfo(ctx context.Context) (*IPBasicInfo, riskFlags, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://ip-api.com/json/?fields=status,query,country,countryCode,isp,org,as,hosting,proxy", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, riskFlags{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, riskFlags{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, riskFlags{}, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var data ipAPIResponse
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, riskFlags{}, err
	}
	if data.Status != "success" {
		return nil, riskFlags{}, fmt.Errorf("status %s: %s", data.Status, strings.TrimSpace(data.Message))
	}
	asn := int64(0)
	if m := asnRe.FindStringSubmatch(data.AS); len(m) > 1 {
		fmt.Sscan(m[1], &asn)
	}
	info := &IPBasicInfo{
		Source:      "ip-api.com",
		IP:          data.IP,
		Country:     data.Country,
		CountryCode: data.CountryCode,
		ASN:         asn,
		Org:         data.Org,
		ISP:         data.ISP,
		Hosting:     data.Hosting,
		Proxy:       data.Proxy,
	}
	return info, riskFlags{proxy: data.Proxy, hosting: data.Hosting}, nil
}

func getPublicIP(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api64.ipify.org", nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	ip := strings.TrimSpace(string(body))
	if net.ParseIP(ip) == nil {
		return "", fmt.Errorf("invalid address %q", ip)
	}
	return ip, nil
}

// --- DNSBL ---

var dnsblZones = []string{"zen.spamhaus.org", "bl.spamcop.net", "dnsbl.sorbs.net"}

func checkDNSBL(ctx context.Context, ip string) dnsblCheckResult {
	resolver := net.DefaultResolver
	return checkDNSBLWithLookup(ctx, ip, resolver.LookupHost)
}

func checkDNSBLWithLookup(ctx context.Context, ip string, lookup func(context.Context, string) ([]string, error)) dnsblCheckResult {
	parts := strings.Split(ip, ".")
	if len(parts) != 4 {
		return dnsblCheckResult{Errors: []string{fmt.Sprintf("invalid IPv4 address %q", ip)}}
	}
	reversed := fmt.Sprintf("%s.%s.%s.%s", parts[3], parts[2], parts[1], parts[0])

	type zoneResult struct {
		zone    string
		listed  bool
		checked bool
		err     error
	}
	results := make([]zoneResult, len(dnsblZones))
	var wg sync.WaitGroup
	for i, zone := range dnsblZones {
		wg.Add(1)
		go func(idx int, zone string) {
			defer wg.Done()
			lookupCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			addresses, err := lookup(lookupCtx, reversed+"."+zone)
			cancel()
			results[idx] = zoneResult{zone: zone, err: err}
			if err == nil {
				results[idx].checked = true
				results[idx].listed = len(addresses) > 0
				return
			}
			var dnsErr *net.DNSError
			if errors.As(err, &dnsErr) && dnsErr.IsNotFound {
				results[idx].checked = true
				results[idx].err = nil
			}
		}(i, zone)
	}
	wg.Wait()

	out := dnsblCheckResult{}
	for _, result := range results {
		if result.checked {
			out.Checked++
			if result.listed {
				out.Listed = append(out.Listed, result.zone)
			}
			continue
		}
		if result.err != nil {
			out.Errors = append(out.Errors, fmt.Sprintf("%s: %v", result.zone, result.err))
		}
	}
	return out
}

func scoreLevel(score int) string {
	switch {
	case score >= 80:
		return "Excellent"
	case score >= 60:
		return "Good"
	case score >= 40:
		return "Fair"
	default:
		return "Poor"
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
