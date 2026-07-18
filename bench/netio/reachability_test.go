package netio

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestProbeReachabilityReturnsStructuredHTTPAndTCPResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error = %v", err)
	}
	defer listener.Close()
	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr == nil {
			_ = conn.Close()
		}
	}()

	targets := []ReachabilityTarget{
		{ID: "web", Category: "website", Protocol: "http", Endpoint: server.URL},
		{ID: "tg", Category: "telegram", Protocol: "tcp", Endpoint: listener.Addr().String()},
	}
	results := ProbeReachability(context.Background(), targets)
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}
	if results[0].ID != "web" || results[0].Status != "reachable" || results[0].HTTPStatus != http.StatusNoContent {
		t.Fatalf("HTTP result = %+v", results[0])
	}
	if results[1].ID != "tg" || results[1].Status != "reachable" || results[1].HTTPStatus != 0 {
		t.Fatalf("TCP result = %+v", results[1])
	}
	if results[0].LatencyMs <= 0 || results[1].LatencyMs <= 0 {
		t.Fatalf("latencies = %f, %f; want > 0", results[0].LatencyMs, results[1].LatencyMs)
	}
}

func TestProbeReachabilityRecordsHTTPErrorStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "forbidden", http.StatusForbidden)
	}))
	defer server.Close()

	results := ProbeReachability(context.Background(), []ReachabilityTarget{{
		ID: "blocked", Category: "website", Protocol: "http", Endpoint: server.URL,
	}})
	if len(results) != 1 || results[0].Status != "http_error" || results[0].HTTPStatus != http.StatusForbidden {
		t.Fatalf("results = %+v, want structured HTTP error", results)
	}
	if !strings.Contains(results[0].Error, "403") {
		t.Fatalf("Error = %q, want HTTP status", results[0].Error)
	}
}

func TestProbeReachabilityValidatesTargetsWithoutDialing(t *testing.T) {
	called := false
	results := probeReachability(context.Background(), []ReachabilityTarget{{
		ID: "bad", Category: "telegram", Protocol: "tcp", Endpoint: "missing-port",
	}}, time.Second, 1, reachabilityDependencies{
		dial: func(context.Context, string, string) (net.Conn, error) {
			called = true
			return nil, errors.New("unexpected")
		},
	})
	if called {
		t.Fatal("dial called for invalid endpoint")
	}
	if len(results) != 1 || results[0].Status != "invalid" || !strings.Contains(results[0].Error, "invalid TCP endpoint") {
		t.Fatalf("results = %+v, want invalid target", results)
	}
}

func TestProbeReachabilityBoundsConcurrencyAndPreservesOrder(t *testing.T) {
	var current atomic.Int32
	var maximum atomic.Int32
	deps := reachabilityDependencies{
		httpDo: func(*http.Request) (*http.Response, error) {
			now := current.Add(1)
			for {
				old := maximum.Load()
				if now <= old || maximum.CompareAndSwap(old, now) {
					break
				}
			}
			time.Sleep(20 * time.Millisecond)
			current.Add(-1)
			return &http.Response{StatusCode: http.StatusNoContent, Body: io.NopCloser(strings.NewReader(""))}, nil
		},
	}
	targets := make([]ReachabilityTarget, 6)
	for idx := range targets {
		targets[idx] = ReachabilityTarget{ID: string(rune('a' + idx)), Category: "website", Protocol: "https", Endpoint: "https://example.com"}
	}
	results := probeReachability(context.Background(), targets, time.Second, 2, deps)
	if got := maximum.Load(); got != 2 {
		t.Fatalf("maximum concurrency = %d, want 2", got)
	}
	for idx := range targets {
		if results[idx].ID != targets[idx].ID || results[idx].Status != "reachable" {
			t.Fatalf("results[%d] = %+v, want target %q reachable", idx, results[idx], targets[idx].ID)
		}
	}
}

func TestProbeReachabilityRecordsPerTargetTimeout(t *testing.T) {
	deps := reachabilityDependencies{
		httpDo: func(req *http.Request) (*http.Response, error) {
			<-req.Context().Done()
			return nil, req.Context().Err()
		},
	}
	results := probeReachability(context.Background(), []ReachabilityTarget{{
		ID: "slow", Category: "website", Protocol: "https", Endpoint: "https://example.com",
	}}, 5*time.Millisecond, 1, deps)
	if len(results) != 1 || results[0].Status != "unreachable" || !strings.Contains(results[0].Error, "timeout") {
		t.Fatalf("results = %+v, want timeout", results)
	}
}

func TestDefaultReachabilityTargetsHaveStableUniqueIDs(t *testing.T) {
	targets := DefaultReachabilityTargets()
	seen := make(map[string]struct{}, len(targets))
	hasWebsite := false
	hasTelegram := false
	for _, target := range targets {
		if _, ok := seen[target.ID]; ok {
			t.Fatalf("duplicate target ID %q", target.ID)
		}
		seen[target.ID] = struct{}{}
		hasWebsite = hasWebsite || target.Category == "website"
		hasTelegram = hasTelegram || target.Category == "telegram"
	}
	if !hasWebsite || !hasTelegram {
		t.Fatalf("targets = %+v, want website and telegram categories", targets)
	}
}
