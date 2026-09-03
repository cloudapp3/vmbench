package netio

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

func TestProbeMailPortsClassifiesDialResults(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus string
	}{
		{name: "open", wantStatus: MailPortStatusOpen},
		{name: "refused", err: fmt.Errorf("dial failed: %w", syscall.ECONNREFUSED), wantStatus: MailPortStatusRefused},
		{name: "timeout", err: context.DeadlineExceeded, wantStatus: MailPortStatusTimeout},
		{name: "DNS timeout", err: &net.DNSError{Err: "i/o timeout", Name: mailPortTarget, IsTimeout: true}, wantStatus: MailPortStatusError},
		{name: "error", err: errors.New("resolver unavailable"), wantStatus: MailPortStatusError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := probeMailPorts(context.Background(), []int{25}, time.Second, func(context.Context, string, string) (net.Conn, error) {
				if tt.err != nil {
					return nil, tt.err
				}
				client, server := net.Pipe()
				_ = server.Close()
				return client, nil
			})
			if len(result) != 1 || result[0].Status != tt.wantStatus {
				t.Fatalf("probeMailPorts() = %+v, want status %q", result, tt.wantStatus)
			}
			if result[0].Port != 25 || result[0].Target != mailPortTarget || result[0].Method != "tcp_connect" || !result[0].Supported {
				t.Fatalf("probeMailPorts() = %+v, want compatible port probe fields", result)
			}
			if tt.err == nil && result[0].Message != "reachable" {
				t.Fatalf("Message = %q, want reachable", result[0].Message)
			}
			if tt.err != nil && !strings.Contains(result[0].Message, tt.err.Error()) {
				t.Fatalf("Message = %q, want dial error %q", result[0].Message, tt.err)
			}
		})
	}
}

func TestProbeMailPortsRunsSequentiallyAndPreservesOrder(t *testing.T) {
	ports := []int{25, 465, 587, 2525}
	var current atomic.Int32
	var maximum atomic.Int32
	var callsMu sync.Mutex
	var calls []string
	dial := func(_ context.Context, _, address string) (net.Conn, error) {
		now := current.Add(1)
		for {
			old := maximum.Load()
			if now <= old || maximum.CompareAndSwap(old, now) {
				break
			}
		}
		callsMu.Lock()
		calls = append(calls, address)
		callsMu.Unlock()
		time.Sleep(5 * time.Millisecond)
		current.Add(-1)
		return nil, errors.New("probe failed")
	}

	results := probeMailPorts(context.Background(), ports, time.Second, dial)
	if got := maximum.Load(); got != 1 {
		t.Fatalf("maximum concurrency = %d, want 1", got)
	}
	if len(results) != len(ports) || len(calls) != len(ports) {
		t.Fatalf("results/calls = %d/%d, want %d", len(results), len(calls), len(ports))
	}
	for idx, port := range ports {
		wantAddress := fmt.Sprintf("%s:%d", mailPortTarget, port)
		if results[idx].Port != port || calls[idx] != wantAddress {
			t.Fatalf("result/call[%d] = %+v/%q, want port %d/address %q", idx, results[idx], calls[idx], port, wantAddress)
		}
	}
}

func TestProbeMailPortsUsesDefaultsForEmptyInput(t *testing.T) {
	results := probeMailPorts(context.Background(), nil, time.Second, func(context.Context, string, string) (net.Conn, error) {
		return nil, errors.New("probe failed")
	})
	ports := DefaultMailPorts()
	if len(results) != len(ports) {
		t.Fatalf("len(results) = %d, want %d", len(results), len(ports))
	}
	for idx := range ports {
		if results[idx].Port != ports[idx] {
			t.Fatalf("results[%d].Port = %d, want %d", idx, results[idx].Port, ports[idx])
		}
	}
}
