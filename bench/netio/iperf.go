package netio

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"

	"github.com/cloudapp3/vmbench/bench"
)

// IperfProbeResult stores one iperf3 probe outcome.
type IperfProbeResult struct {
	Host          string        `json:"host"`
	BitsPerSecond float64       `json:"bits_per_second,omitempty"`
	Elapsed       time.Duration `json:"elapsed,omitempty"`
}

func (r IperfProbeResult) ThroughputMbitPerSec() float64 {
	if r.BitsPerSecond <= 0 {
		return 0
	}
	return r.BitsPerSecond / 1_000_000
}

// ProbeIperf runs a single iperf3 TCP download benchmark against host.
func ProbeIperf(ctx context.Context, host string, duration int) (*IperfProbeResult, error) {
	if duration <= 0 {
		duration = 10
	}
	timeout := time.Duration(duration)*time.Second + 20*time.Second
	if timeout < 30*time.Second {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()
	out, err := exec.CommandContext(ctx, "iperf3", "-c", host, "-J", "-t", fmt.Sprintf("%d", duration)).CombinedOutput()
	elapsed := time.Since(start)
	if err != nil {
		return nil, fmt.Errorf("iperf3: %w: %s", err, out)
	}

	var result struct {
		End struct {
			SumReceived struct {
				BitsPerSecond float64 `json:"bits_per_second"`
			} `json:"sum_received"`
		} `json:"end"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, err
	}
	return &IperfProbeResult{Host: host, BitsPerSecond: result.End.SumReceived.BitsPerSecond, Elapsed: elapsed}, nil
}

type iperfWorkload struct {
	host     string
	duration int
	lastBPS  float64
}

// NewIperfWorkload creates a benchmark workload wrapper around iperf3.
func NewIperfWorkload(host string, duration int) bench.Workload {
	return &iperfWorkload{host: host, duration: duration}
}

func (w *iperfWorkload) Name() string        { return "Network (iperf3 → " + w.host + ")" }
func (w *iperfWorkload) Category() string    { return bench.CategoryNetwork }
func (w *iperfWorkload) Description() string { return "iperf3 TCP bandwidth test" }
func (w *iperfWorkload) Validate() error     { return nil }
func (w *iperfWorkload) SkipWarmup() bool    { return true }
func (w *iperfWorkload) MaxIterations() int  { return 1 }
func (w *iperfWorkload) Throughput(int64, time.Duration) (float64, string) {
	return w.lastBPS / 1_000_000, "Mbit/s"
}

func (w *iperfWorkload) Run(ctx context.Context) (time.Duration, int64, error) {
	result, err := ProbeIperf(ctx, w.host, w.duration)
	if err != nil {
		return 0, 0, err
	}
	w.lastBPS = result.BitsPerSecond
	return result.Elapsed, int64(result.BitsPerSecond / 8), nil
}
