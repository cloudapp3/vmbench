package netio

import (
	"context"
	"errors"
	"net"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestPingTargetCountsTCPRSTAsResponse(t *testing.T) {
	for _, errno := range []syscall.Errno{syscall.ECONNREFUSED, syscall.ECONNRESET} {
		t.Run(errno.Error(), func(t *testing.T) {
			rstErr := &net.OpError{
				Op:  "dial",
				Net: "tcp",
				Err: errno,
			}
			result := pingTargetWithDial(context.Background(), PingTarget{
				ID:       "closed-port",
				Name:     "Closed port",
				Endpoint: "192.0.2.1",
				Port:     80,
			}, func(context.Context, string, string) (net.Conn, error) {
				return nil, rstErr
			})

			if result.Status != "ok" {
				t.Fatalf("Status = %q, want ok for TCP RST response", result.Status)
			}
			if result.ConnectionState != PingConnectionStateRefused {
				t.Fatalf("ConnectionState = %q, want %q", result.ConnectionState, PingConnectionStateRefused)
			}
			if result.Sent != pingProbes || result.Received != pingProbes || result.PacketLoss != 0 {
				t.Fatalf("probe counts = sent %d received %d loss %.1f, want %d/%d/0", result.Sent, result.Received, result.PacketLoss, pingProbes, pingProbes)
			}
			if !strings.Contains(result.Message, "TCP RST") || !strings.Contains(result.Message, rstErr.Error()) {
				t.Fatalf("Message = %q, want RST explanation and raw dial error", result.Message)
			}
		})
	}
}

func TestPingTargetKeepsTimeoutAndUnreachableAsLoss(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{name: "timeout", err: context.DeadlineExceeded},
		{name: "host unreachable", err: syscall.EHOSTUNREACH},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := pingTargetWithDial(context.Background(), PingTarget{
				ID:       "no-response",
				Name:     "No response",
				Endpoint: "192.0.2.1",
				Port:     80,
			}, func(context.Context, string, string) (net.Conn, error) {
				return nil, tt.err
			})

			if result.Status != "error" {
				t.Fatalf("Status = %q, want error", result.Status)
			}
			if result.ConnectionState != PingConnectionStateNoResponse {
				t.Fatalf("ConnectionState = %q, want %q", result.ConnectionState, PingConnectionStateNoResponse)
			}
			if result.Received != 0 || result.PacketLoss != 100 {
				t.Fatalf("probe counts = received %d loss %.1f, want 0/100", result.Received, result.PacketLoss)
			}
			if !strings.Contains(result.Message, tt.err.Error()) {
				t.Fatalf("Message = %q, want raw error %q", result.Message, tt.err)
			}
		})
	}
}

func TestPingTargetReportsMixedOpenAndRefusedResponses(t *testing.T) {
	calls := 0
	result := pingTargetWithDial(context.Background(), PingTarget{
		ID:       "mixed",
		Name:     "Mixed",
		Endpoint: "192.0.2.1",
		Port:     80,
	}, func(context.Context, string, string) (net.Conn, error) {
		calls++
		if calls%2 == 0 {
			return nil, syscall.ECONNREFUSED
		}
		client, peer := net.Pipe()
		_ = peer.Close()
		return client, nil
	})

	if result.Status != "ok" || result.ConnectionState != PingConnectionStateMixed {
		t.Fatalf("result = %+v, want ok mixed response", result)
	}
	if result.Received != pingProbes || result.PacketLoss != 0 {
		t.Fatalf("probe counts = received %d loss %.1f, want %d/0", result.Received, result.PacketLoss, pingProbes)
	}
}

func TestProbePingTargetsReturnsResultsAndErrorWhenAllFail(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	results, err := ProbePingTargets(ctx, []PingTarget{
		{ID: "one", Name: "One", Endpoint: "127.0.0.1", Port: 1},
		{ID: "two", Name: "Two", Endpoint: "127.0.0.1", Port: 1},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ProbePingTargets() error = %v, want context.Canceled", err)
	}
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}
	for i, result := range results {
		if result.Status != "error" || result.Message == "" {
			t.Fatalf("results[%d] = %+v, want structured error result", i, result)
		}
		if result.ProbeProtocol != "tcp-connect" || result.ProbeTool != "go-net-dialer" {
			t.Fatalf("results[%d] probe evidence = %q/%q", i, result.ProbeProtocol, result.ProbeTool)
		}
	}
}

func TestProbePingTargetsKeepsPartialResultsWithoutAggregateError(t *testing.T) {
	targets := []PingTarget{
		{ID: "ok", Name: "OK", Endpoint: "ok.example"},
		{ID: "failed", Name: "Failed", Endpoint: "failed.example"},
	}
	results, err := probePingTargets(context.Background(), targets, func(_ context.Context, target PingTarget) PingProbeResult {
		result := PingProbeResult{ID: target.ID, Name: target.Name, Target: target.Endpoint, Sent: pingProbes}
		if target.ID == "ok" {
			result.Status = "ok"
			result.Received = pingProbes
			result.AvgLatencyMs = 12.5
			return result
		}
		result.Status = "error"
		result.Message = "dial failed"
		result.PacketLoss = 100
		return result
	})
	if err != nil {
		t.Fatalf("probePingTargets() error = %v, want nil for partial success", err)
	}
	if len(results) != 2 || results[0].Status != "ok" || results[1].Status != "error" {
		t.Fatalf("results = %+v, want retained ok and error results", results)
	}
}

func TestProbePingNodesReturnsStructuredErrorWhenAllFail(t *testing.T) {
	nodes := []SpeedNode{{Name: "One", Region: "test"}, {Name: "Two", Region: "test"}}
	results, err := probePingNodes(context.Background(), nodes, func(_ context.Context, node SpeedNode) nodePingResult {
		return nodePingResult{name: node.Name, region: node.Region, host: "failed.example", loss: 100, err: errors.New("dial failed")}
	})
	if err == nil || !strings.Contains(err.Error(), "all 2 ping targets failed") {
		t.Fatalf("probePingNodes() error = %v, want aggregate all-failed error", err)
	}
	if len(results) != 2 || results[0].Message != "dial failed" || results[1].Message != "dial failed" {
		t.Fatalf("results = %+v, want per-node errors retained", results)
	}
}

func TestPingWorkloadReturnsErrorAfterBuildingFailureDetail(t *testing.T) {
	w := &pingWorkload{
		probeNodes: func(context.Context, []SpeedNode) ([]PingProbeResult, error) {
			results := []PingProbeResult{
				{Name: "One", Status: "error", Message: "dial failed", Sent: pingProbes, PacketLoss: 100},
				{Name: "Two", Status: "error", Message: "timeout", Sent: pingProbes, PacketLoss: 100},
			}
			return results, errors.New("all 2 ping targets failed: dial failed")
		},
	}

	elapsed, processed, err := w.Run(context.Background())
	if err == nil {
		t.Fatal("Run() error = nil, want all-failed error")
	}
	if elapsed < 0 || processed != 0 {
		t.Fatalf("Run() = %s, %d, %v; want zero successful targets", elapsed, processed, err)
	}
	if !strings.Contains(w.Detail(), "One=FAIL") || !strings.Contains(w.Detail(), "loss=100.0%") {
		t.Fatalf("Detail() = %q, want structured failed-node detail", w.Detail())
	}

	_, _, cachedErr := w.Run(context.Background())
	if cachedErr == nil || cachedErr.Error() != err.Error() {
		t.Fatalf("cached Run() error = %v, want %v", cachedErr, err)
	}
}

func TestPingWorkloadAllowsPartialSuccess(t *testing.T) {
	w := &pingWorkload{
		probeNodes: func(context.Context, []SpeedNode) ([]PingProbeResult, error) {
			return []PingProbeResult{
				{Name: "One", Status: "ok", AvgLatencyMs: 10, Sent: pingProbes, Received: pingProbes},
				{Name: "Two", Status: "error", Message: "dial failed", Sent: pingProbes, PacketLoss: 100},
			}, nil
		},
	}

	_, processed, err := w.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error = %v, want nil for partial success", err)
	}
	if processed != 1 {
		t.Fatalf("Run() processed = %d, want 1 successful target", processed)
	}
	throughput, unit := w.Throughput(0, time.Second)
	if throughput != 10 || unit != "ms avg" {
		t.Fatalf("Throughput() = %.1f %s, want 10 ms avg", throughput, unit)
	}
}
