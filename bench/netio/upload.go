package netio

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/cloudapp3/vmbench/bench"
)

var (
	cfUploadURL = "https://speed.cloudflare.com/__up"
	uploadSize  = 50 * 1024 * 1024 // 50 MB
)

// UploadProbeResult stores one Cloudflare upload test outcome.
type UploadProbeResult struct {
	Bytes   int64         `json:"bytes,omitempty"`
	Elapsed time.Duration `json:"elapsed,omitempty"`
}

func (r UploadProbeResult) ThroughputMiBPerSec() float64 {
	if r.Elapsed <= 0 {
		return 0
	}
	return float64(r.Bytes) / r.Elapsed.Seconds() / (1024 * 1024)
}

// ProbeUpload measures upload throughput via Cloudflare speed test endpoint.
func ProbeUpload(ctx context.Context) (*UploadProbeResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	if uploadSize <= 0 {
		return nil, fmt.Errorf("upload: invalid size %d", uploadSize)
	}
	body := io.LimitReader(zeroReader{}, int64(uploadSize))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfUploadURL, body)
	if err != nil {
		return nil, fmt.Errorf("upload: %w", err)
	}
	req.ContentLength = int64(uploadSize)
	req.Header.Set("Content-Type", "application/octet-stream")

	start := time.Now()
	resp, err := http.DefaultClient.Do(req)
	elapsed := time.Since(start)
	if err != nil {
		return nil, fmt.Errorf("upload: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, fmt.Errorf("upload: HTTP %d", resp.StatusCode)
	}
	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		return nil, fmt.Errorf("upload response: %w", err)
	}

	return &UploadProbeResult{Bytes: int64(uploadSize), Elapsed: elapsed}, nil
}

// uploadWorkload measures upload throughput via Cloudflare speed test endpoint.
type uploadWorkload struct {
	bps        float64
	totalBytes int64
	elapsed    time.Duration
	detail     string
}

// NewUploadWorkload creates an upload speed benchmark.
func NewUploadWorkload() bench.Workload {
	return &uploadWorkload{}
}

func (w *uploadWorkload) Name() string                     { return "Net Upload" }
func (w *uploadWorkload) Category() string                 { return bench.CategoryNetwork }
func (w *uploadWorkload) Description() string              { return "Upload speed via Cloudflare (50MB)" }
func (*uploadWorkload) ProcessedKind() bench.ProcessedKind { return bench.ProcessedBytes }
func (w *uploadWorkload) Validate() error                  { return nil }
func (w *uploadWorkload) SkipWarmup() bool                 { return true }
func (w *uploadWorkload) MaxIterations() int               { return 1 }

func (w *uploadWorkload) Throughput(int64, time.Duration) (float64, string) {
	return w.bps / (1024 * 1024), "MiB/s"
}

func (w *uploadWorkload) Detail() string { return w.detail }

func (w *uploadWorkload) Run(ctx context.Context) (time.Duration, int64, error) {
	if w.totalBytes > 0 {
		return w.elapsed, w.totalBytes, nil
	}
	result, err := ProbeUpload(ctx)
	if err != nil {
		return 0, 0, err
	}
	w.elapsed = result.Elapsed
	w.totalBytes = result.Bytes
	if result.Elapsed.Seconds() > 0 {
		w.bps = float64(result.Bytes) / result.Elapsed.Seconds()
	}
	w.detail = fmt.Sprintf("%.1f MiB/s (%.1f MiB in %.1fs)", result.ThroughputMiBPerSec(), float64(result.Bytes)/(1024*1024), result.Elapsed.Seconds())
	return result.Elapsed, result.Bytes, nil
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}
