package nodecatalog

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// HealthOptions bounds lightweight node checks. HTTP nodes are checked with
// HEAD; other nodes use DNS and, when a port exists, a TCP connection.
type HealthOptions struct {
	Timeout     time.Duration
	Concurrency int
	Filter      Filter
	Client      *http.Client
	Resolver    *net.Resolver
}

// HealthResult is one node's non-benchmark availability result.
type HealthResult struct {
	NodeID     string        `json:"node_id"`
	Status     string        `json:"status"`
	Method     string        `json:"method"`
	Endpoint   string        `json:"endpoint"`
	HTTPStatus int           `json:"http_status,omitempty"`
	Addresses  []string      `json:"addresses,omitempty"`
	Latency    time.Duration `json:"-"`
	LatencyMs  float64       `json:"latency_ms,omitempty"`
	Error      string        `json:"error,omitempty"`
}

// CheckHealth performs bounded checks while preserving manifest order.
func CheckHealth(ctx context.Context, manifest Manifest, options HealthOptions) []HealthResult {
	nodes := manifest.Select(options.Filter)
	results := make([]HealthResult, len(nodes))
	if len(nodes) == 0 {
		return results
	}
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	workers := options.Concurrency
	if workers <= 0 {
		workers = 8
	}
	if workers > 32 {
		workers = 32
	}
	if workers > len(nodes) {
		workers = len(nodes)
	}

	jobs := make(chan int, len(nodes))
	for i := range nodes {
		jobs <- i
	}
	close(jobs)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				checkCtx, cancel := context.WithTimeout(ctx, timeout)
				result := checkNode(checkCtx, nodes[index], options)
				result.LatencyMs = float64(result.Latency) / float64(time.Millisecond)
				results[index] = result
				cancel()
			}
		}()
	}
	wg.Wait()
	return results
}

func checkNode(ctx context.Context, node Node, options HealthOptions) HealthResult {
	if node.Protocol == "http" || node.Protocol == "https" {
		return checkHTTP(ctx, node, options.Client)
	}
	return checkDNSAndTCP(ctx, node, options.Resolver)
}

func checkHTTP(ctx context.Context, node Node, client *http.Client) HealthResult {
	result := HealthResult{NodeID: node.ID, Method: "HEAD", Endpoint: node.URL}
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, node.URL, nil)
	if err != nil {
		result.Status = "error"
		result.Error = err.Error()
		return result
	}
	start := time.Now()
	resp, err := client.Do(req)
	result.Latency = time.Since(start)
	if err != nil {
		result.Status = "error"
		result.Error = err.Error()
		return result
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
	result.HTTPStatus = resp.StatusCode
	if resp.StatusCode >= http.StatusInternalServerError {
		result.Status = "error"
		result.Error = fmt.Sprintf("HTTP %d", resp.StatusCode)
		return result
	}
	result.Status = "ok"
	return result
}

func checkDNSAndTCP(ctx context.Context, node Node, resolver *net.Resolver) HealthResult {
	method := "DNS"
	if node.Port > 0 {
		method = "DNS+TCP"
	}
	result := HealthResult{NodeID: node.ID, Method: method, Endpoint: node.Endpoint}
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	network := "ip"
	switch node.IPFamily {
	case "v4":
		network = "ip4"
	case "v6":
		network = "ip6"
	}
	start := time.Now()
	addresses, err := resolver.LookupIP(ctx, network, node.Endpoint)
	if err != nil {
		result.Latency = time.Since(start)
		result.Status = "error"
		result.Error = err.Error()
		return result
	}
	for _, address := range addresses {
		result.Addresses = append(result.Addresses, address.String())
	}
	if node.Port == 0 {
		result.Latency = time.Since(start)
		result.Status = "ok"
		return result
	}
	dialNetwork := "tcp"
	if node.IPFamily == "v4" {
		dialNetwork = "tcp4"
	} else if node.IPFamily == "v6" {
		dialNetwork = "tcp6"
	}
	dialer := &net.Dialer{}
	conn, err := dialer.DialContext(ctx, dialNetwork, net.JoinHostPort(node.Endpoint, strconv.Itoa(node.Port)))
	result.Latency = time.Since(start)
	if err != nil {
		result.Status = "error"
		result.Error = err.Error()
		return result
	}
	_ = conn.Close()
	result.Status = "ok"
	return result
}

// EndpointForDisplay returns a compact URL or host:port representation.
func EndpointForDisplay(node Node) string {
	if strings.TrimSpace(node.URL) != "" {
		if parsed, err := url.Parse(node.URL); err == nil {
			return parsed.Host + parsed.Path
		}
		return node.URL
	}
	if node.Port > 0 {
		return net.JoinHostPort(node.Endpoint, strconv.Itoa(node.Port))
	}
	return node.Endpoint
}
