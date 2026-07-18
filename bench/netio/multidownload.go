package netio

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/cloudapp3/vmbench/bench"
)

const multiThreads = 4

var cfDownloadURL = "https://speed.cloudflare.com/__down?bytes=50000000"

// MultiDownloadProbeResult stores the aggregate Cloudflare concurrent download outcome.
type MultiDownloadProbeResult struct {
	Threads    int           `json:"threads"`
	TotalBytes int64         `json:"total_bytes,omitempty"`
	Elapsed    time.Duration `json:"elapsed,omitempty"`
}

func (r MultiDownloadProbeResult) ThroughputMiBPerSec() float64 {
	if r.Elapsed <= 0 {
		return 0
	}
	return float64(r.TotalBytes) / r.Elapsed.Seconds() / (1024 * 1024)
}

// ProbeMultiDownload measures aggregate download throughput using concurrent goroutines.
func ProbeMultiDownload(ctx context.Context) (*MultiDownloadProbeResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	start := time.Now()
	var totalBytes int64
	var mu sync.Mutex
	var errsMu sync.Mutex
	var errs []string

	var wg sync.WaitGroup
	for i := 0; i < multiThreads; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfDownloadURL, nil)
			if err != nil {
				errsMu.Lock()
				errs = append(errs, err.Error())
				errsMu.Unlock()
				return
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				errsMu.Lock()
				errs = append(errs, err.Error())
				errsMu.Unlock()
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				_, _ = io.Copy(io.Discard, resp.Body)
				errsMu.Lock()
				errs = append(errs, fmt.Sprintf("HTTP %d", resp.StatusCode))
				errsMu.Unlock()
				return
			}
			n, err := io.Copy(io.Discard, resp.Body)
			if err != nil {
				errsMu.Lock()
				errs = append(errs, err.Error())
				errsMu.Unlock()
				return
			}
			mu.Lock()
			totalBytes += n
			mu.Unlock()
		}()
	}
	wg.Wait()
	elapsed := time.Since(start)
	if len(errs) > 0 {
		return nil, fmt.Errorf("multi download: %s", errs[0])
	}
	return &MultiDownloadProbeResult{Threads: multiThreads, TotalBytes: totalBytes, Elapsed: elapsed}, nil
}

// multiDownloadWorkload measures aggregate download throughput using N concurrent goroutines.
type multiDownloadWorkload struct {
	bps        float64
	totalBytes int64
	elapsed    time.Duration
	detail     string
}

// NewMultiDownloadWorkload creates a multi-threaded download benchmark.
func NewMultiDownloadWorkload() bench.Workload {
	return &multiDownloadWorkload{}
}

func (w *multiDownloadWorkload) Name() string     { return "Net Multi-Thread Download" }
func (w *multiDownloadWorkload) Category() string { return bench.CategoryNetwork }
func (w *multiDownloadWorkload) Description() string {
	return fmt.Sprintf("Concurrent download (%d threads, Cloudflare)", multiThreads)
}
func (*multiDownloadWorkload) ProcessedKind() bench.ProcessedKind { return bench.ProcessedBytes }
func (w *multiDownloadWorkload) Validate() error                  { return nil }
func (w *multiDownloadWorkload) SkipWarmup() bool                 { return true }
func (w *multiDownloadWorkload) MaxIterations() int {
	return 1
}

func (w *multiDownloadWorkload) Throughput(int64, time.Duration) (float64, string) {
	return w.bps / (1024 * 1024), "MiB/s"
}

func (w *multiDownloadWorkload) Detail() string { return w.detail }

func (w *multiDownloadWorkload) Run(ctx context.Context) (time.Duration, int64, error) {
	if w.totalBytes > 0 {
		return w.elapsed, w.totalBytes, nil
	}
	result, err := ProbeMultiDownload(ctx)
	if err != nil {
		return 0, 0, err
	}
	w.elapsed = result.Elapsed
	w.totalBytes = result.TotalBytes
	if result.Elapsed.Seconds() > 0 {
		w.bps = float64(result.TotalBytes) / result.Elapsed.Seconds()
	}
	mib := float64(result.TotalBytes) / (1024 * 1024)
	mibPerSec := result.ThroughputMiBPerSec()
	w.detail = fmt.Sprintf("%d threads × 50MB, total %.1f MiB in %.1fs = %.1f MiB/s", result.Threads, mib, result.Elapsed.Seconds(), mibPerSec)
	return result.Elapsed, result.TotalBytes, nil
}
