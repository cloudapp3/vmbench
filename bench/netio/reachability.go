package netio

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	reachabilityConcurrency   = 4
	reachabilityTargetTimeout = 10 * time.Second

	ReachabilityStatusReachable   = "reachable"
	ReachabilityStatusUnreachable = "unreachable"
	ReachabilityStatusHTTPError   = "http_error"
	ReachabilityStatusInvalid     = "invalid"
)

// ReachabilityTarget describes one built-in or caller-supplied endpoint.
type ReachabilityTarget struct {
	ID       string `json:"id"`
	Category string `json:"category"`
	Protocol string `json:"protocol"`
	Endpoint string `json:"endpoint"`
}

// ReachabilityProbeResult stores transport evidence for one endpoint.
type ReachabilityProbeResult struct {
	ID         string  `json:"id"`
	Category   string  `json:"category"`
	Protocol   string  `json:"protocol"`
	Endpoint   string  `json:"endpoint"`
	Status     string  `json:"status"`
	LatencyMs  float64 `json:"latency_ms,omitempty"`
	HTTPStatus int     `json:"http_status,omitempty"`
	Error      string  `json:"error,omitempty"`
}

type reachabilityDependencies struct {
	httpDo func(*http.Request) (*http.Response, error)
	dial   func(context.Context, string, string) (net.Conn, error)
}

var defaultReachabilityTargets = []ReachabilityTarget{
	{ID: "google", Category: "website", Protocol: "https", Endpoint: "https://www.google.com/generate_204"},
	{ID: "github", Category: "website", Protocol: "https", Endpoint: "https://github.com/"},
	{ID: "cloudflare", Category: "website", Protocol: "https", Endpoint: "https://www.cloudflare.com/cdn-cgi/trace"},
	{ID: "telegram_dc1", Category: "telegram", Protocol: "tcp", Endpoint: "149.154.175.53:443"},
	{ID: "telegram_dc2", Category: "telegram", Protocol: "tcp", Endpoint: "149.154.167.51:443"},
	{ID: "telegram_dc3", Category: "telegram", Protocol: "tcp", Endpoint: "149.154.175.100:443"},
	{ID: "telegram_dc4", Category: "telegram", Protocol: "tcp", Endpoint: "149.154.167.91:443"},
	{ID: "telegram_dc5", Category: "telegram", Protocol: "tcp", Endpoint: "91.108.56.130:443"},
}

// DefaultReachabilityTargets returns a copy of the built-in website and
// Telegram DC targets.
func DefaultReachabilityTargets() []ReachabilityTarget {
	return append([]ReachabilityTarget(nil), defaultReachabilityTargets...)
}

// ProbeReachability checks targets with a per-target timeout and bounded
// concurrency. Results retain the input order.
func ProbeReachability(ctx context.Context, targets []ReachabilityTarget) []ReachabilityProbeResult {
	dialer := &net.Dialer{}
	return probeReachability(ctx, targets, reachabilityTargetTimeout, reachabilityConcurrency, reachabilityDependencies{
		httpDo: http.DefaultClient.Do,
		dial:   dialer.DialContext,
	})
}

// ProbeDefaultReachability checks the built-in website and Telegram targets.
func ProbeDefaultReachability(ctx context.Context) []ReachabilityProbeResult {
	return ProbeReachability(ctx, DefaultReachabilityTargets())
}

func probeReachability(ctx context.Context, targets []ReachabilityTarget, timeout time.Duration, concurrency int, deps reachabilityDependencies) []ReachabilityProbeResult {
	if ctx == nil {
		ctx = context.Background()
	}
	if timeout <= 0 {
		timeout = reachabilityTargetTimeout
	}
	if concurrency <= 0 {
		concurrency = 1
	}
	if concurrency > len(targets) {
		concurrency = len(targets)
	}
	results := make([]ReachabilityProbeResult, len(targets))
	if len(targets) == 0 {
		return results
	}

	jobs := make(chan int, len(targets))
	for idx := range targets {
		jobs <- idx
	}
	close(jobs)

	var wg sync.WaitGroup
	for range concurrency {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range jobs {
				targetCtx, cancel := context.WithTimeout(ctx, timeout)
				results[idx] = probeReachabilityTarget(targetCtx, targets[idx], deps)
				cancel()
			}
		}()
	}
	wg.Wait()
	return results
}

func probeReachabilityTarget(ctx context.Context, target ReachabilityTarget, deps reachabilityDependencies) ReachabilityProbeResult {
	target.ID = strings.TrimSpace(target.ID)
	target.Category = strings.ToLower(strings.TrimSpace(target.Category))
	target.Protocol = strings.ToLower(strings.TrimSpace(target.Protocol))
	target.Endpoint = strings.TrimSpace(target.Endpoint)
	result := ReachabilityProbeResult{
		ID:       target.ID,
		Category: target.Category,
		Protocol: target.Protocol,
		Endpoint: target.Endpoint,
		Status:   ReachabilityStatusInvalid,
	}
	if target.ID == "" {
		result.Error = "target ID is required"
		return result
	}
	if target.Category == "" {
		result.Error = "target category is required"
		return result
	}
	if target.Endpoint == "" {
		result.Error = "target endpoint is required"
		return result
	}

	switch target.Protocol {
	case "http", "https":
		return probeReachabilityHTTP(ctx, target, result, deps)
	case "tcp":
		return probeReachabilityTCP(ctx, target, result, deps)
	default:
		result.Error = fmt.Sprintf("unsupported protocol %q", target.Protocol)
		return result
	}
}

func probeReachabilityHTTP(ctx context.Context, target ReachabilityTarget, result ReachabilityProbeResult, deps reachabilityDependencies) ReachabilityProbeResult {
	parsed, err := url.ParseRequestURI(target.Endpoint)
	if err != nil || parsed.Host == "" || parsed.Scheme != target.Protocol {
		result.Error = "invalid HTTP endpoint"
		return result
	}
	if deps.httpDo == nil {
		result.Error = "HTTP probe unavailable"
		return result
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.Endpoint, nil)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	req.Header.Set("User-Agent", ua)
	start := time.Now()
	resp, err := deps.httpDo(req)
	result.LatencyMs = durationMilliseconds(time.Since(start))
	if err != nil {
		result.Status = ReachabilityStatusUnreachable
		result.Error = reachabilityError(ctx, err)
		return result
	}
	defer resp.Body.Close()
	result.HTTPStatus = resp.StatusCode
	if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusBadRequest {
		result.Status = ReachabilityStatusReachable
		return result
	}
	result.Status = ReachabilityStatusHTTPError
	result.Error = fmt.Sprintf("unexpected HTTP status %d", resp.StatusCode)
	return result
}

func probeReachabilityTCP(ctx context.Context, target ReachabilityTarget, result ReachabilityProbeResult, deps reachabilityDependencies) ReachabilityProbeResult {
	if _, _, err := net.SplitHostPort(target.Endpoint); err != nil {
		result.Error = "invalid TCP endpoint: " + err.Error()
		return result
	}
	if deps.dial == nil {
		result.Error = "TCP probe unavailable"
		return result
	}
	start := time.Now()
	conn, err := deps.dial(ctx, "tcp", target.Endpoint)
	result.LatencyMs = durationMilliseconds(time.Since(start))
	if err != nil {
		result.Status = ReachabilityStatusUnreachable
		result.Error = reachabilityError(ctx, err)
		return result
	}
	_ = conn.Close()
	result.Status = ReachabilityStatusReachable
	return result
}

func reachabilityError(ctx context.Context, err error) string {
	if ctxErr := ctx.Err(); ctxErr != nil {
		if errors.Is(ctxErr, context.DeadlineExceeded) {
			return "timeout: " + ctxErr.Error()
		}
		return "canceled: " + ctxErr.Error()
	}
	return err.Error()
}

func durationMilliseconds(value time.Duration) float64 {
	if value <= 0 {
		return 0
	}
	return float64(value) / float64(time.Millisecond)
}
