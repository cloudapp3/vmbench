package netio

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/cloudapp3/vmbench/bench"
)

// DownloadProbeResult stores one HTTP download test outcome.
type DownloadProbeResult struct {
	Node    SpeedNode     `json:"node"`
	Bytes   int64         `json:"bytes,omitempty"`
	Elapsed time.Duration `json:"elapsed,omitempty"`
}

func (r DownloadProbeResult) ThroughputMiBPerSec() float64 {
	if r.Elapsed <= 0 {
		return 0
	}
	return float64(r.Bytes) / r.Elapsed.Seconds() / (1024 * 1024)
}

// ProbeDownload measures HTTP download throughput from a single node.
func ProbeDownload(ctx context.Context, node SpeedNode) (*DownloadProbeResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, node.TestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", node.Name, err)
	}
	// Keep traffic_bytes enforceable as a wire-test budget. Disabling automatic
	// compression also prevents a small encoded response from expanding past it.
	req.Header.Set("Accept-Encoding", "identity")
	// Some speedtest endpoints gate on a browser-like User-Agent.
	req.Header.Set("User-Agent", ua)

	start := time.Now()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", node.Name, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download %s: HTTP %d", node.Name, resp.StatusCode)
	}

	reader := io.Reader(resp.Body)
	if node.TrafficBytes > 0 {
		reader = io.LimitReader(resp.Body, node.TrafficBytes)
	}
	n, copyErr := io.Copy(io.Discard, reader)
	elapsed := time.Since(start)
	if copyErr != nil && ctx.Err() != nil {
		if n > 0 {
			return &DownloadProbeResult{Node: node, Bytes: n, Elapsed: elapsed}, nil
		}
		return nil, fmt.Errorf("download %s: timeout", node.Name)
	}
	if copyErr != nil {
		return nil, fmt.Errorf("download %s: %w", node.Name, copyErr)
	}
	// An empty 200 body is a failed measurement, not a zero-throughput result:
	// Ookla-style download endpoints answer bare GETs with no payload.
	if n == 0 {
		return nil, fmt.Errorf("download %s: endpoint returned no data", node.Name)
	}
	return &DownloadProbeResult{Node: node, Bytes: n, Elapsed: elapsed}, nil
}

// downloadWorkload measures HTTP download throughput from a single node.
type downloadWorkload struct {
	node          SpeedNode
	cachedElapsed time.Duration
	cachedBytes   int64
	bps           float64 // bytes per second
}

// NewDownloadWorkload creates a download benchmark for the given node.
func NewDownloadWorkload(node SpeedNode) bench.Workload {
	return &downloadWorkload{node: node}
}

func (w *downloadWorkload) Name() string {
	return fmt.Sprintf("Net Download (%s)", w.node.Name)
}

func (w *downloadWorkload) Category() string { return bench.CategoryNetwork }
func (w *downloadWorkload) Description() string {
	return fmt.Sprintf("HTTP download from %s [%s]", w.node.Name, w.node.Region)
}
func (*downloadWorkload) ProcessedKind() bench.ProcessedKind { return bench.ProcessedBytes }
func (w *downloadWorkload) Validate() error                  { return nil }
func (w *downloadWorkload) SkipWarmup() bool                 { return true }
func (w *downloadWorkload) MaxIterations() int {
	return 1
}

func (w *downloadWorkload) Throughput(int64, time.Duration) (float64, string) {
	return w.bps / (1024 * 1024), "MiB/s"
}

func (w *downloadWorkload) Run(ctx context.Context) (time.Duration, int64, error) {
	if w.cachedBytes > 0 && w.cachedElapsed > 0 {
		return w.cachedElapsed, w.cachedBytes, nil
	}
	result, err := ProbeDownload(ctx, w.node)
	if err != nil {
		return 0, 0, err
	}
	w.cachedElapsed = result.Elapsed
	w.cachedBytes = result.Bytes
	if result.Elapsed.Seconds() > 0 {
		w.bps = float64(result.Bytes) / result.Elapsed.Seconds()
	}
	return result.Elapsed, result.Bytes, nil
}
