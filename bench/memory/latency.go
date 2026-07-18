package memory

import (
	"context"
	"errors"
	"time"

	"github.com/cloudapp3/vmbench/bench"
	"github.com/cloudapp3/vmbench/bench/common"
)

const defaultLatencyBytes = 64 << 20

// LatencyWorkload benchmarks pointer-chasing latency over a 64 MiB ring.
type LatencyWorkload struct {
	next          []int
	steps         int
	expectedFinal int
	lastFinal     int
}

// NewLatencyWorkload returns the default memory latency workload.
func NewLatencyWorkload() bench.Workload {
	return newLatencyWorkload(defaultLatencyBytes)
}

func newLatencyWorkload(sizeBytes int) *LatencyWorkload {
	count := sizeBytes / 8
	if count < 1024 {
		count = 1024
	}
	indices := make([]int, count)
	for idx := range indices {
		indices[idx] = idx
	}
	rng := common.NewRand()
	for i := len(indices) - 1; i > 0; i-- {
		j := rng.Intn(i + 1)
		indices[i], indices[j] = indices[j], indices[i]
	}
	next := make([]int, count)
	for idx := range indices {
		next[indices[idx]] = indices[(idx+1)%len(indices)]
	}
	steps := count * 4
	current := 0
	for step := 0; step < steps; step++ {
		current = next[current]
	}
	return &LatencyWorkload{next: next, steps: steps, expectedFinal: current}
}

func (w *LatencyWorkload) Name() string { return "Mem Latency" }

func (w *LatencyWorkload) Category() string { return bench.CategoryMemory }

func (w *LatencyWorkload) Description() string {
	return "Measures pointer-chasing latency over a randomized 64 MiB linked index ring."
}

func (*LatencyWorkload) ProcessedKind() bench.ProcessedKind { return bench.ProcessedOperations }

func (w *LatencyWorkload) Clone() bench.Workload {
	cp := *w
	cp.lastFinal = 0
	return &cp
}

func (w *LatencyWorkload) Run(ctx context.Context) (time.Duration, int64, error) {
	current := 0
	started := time.Now()
	for step := 0; step < w.steps; step++ {
		if step%16384 == 0 {
			select {
			case <-ctx.Done():
				return 0, 0, ctx.Err()
			default:
			}
		}
		current = w.next[current]
	}
	w.lastFinal = current
	common.ConsumeUint64(uint64(current))
	return time.Since(started), int64(w.steps), nil
}

func (w *LatencyWorkload) Validate() error {
	if w.lastFinal != w.expectedFinal {
		return errors.New("latency pointer chase mismatch")
	}
	return nil
}

func (w *LatencyWorkload) Throughput(processed int64, elapsed time.Duration) (float64, string) {
	if processed <= 0 || elapsed <= 0 {
		return 0, "ops/s"
	}
	return float64(processed) / elapsed.Seconds(), "ops/s"
}

func (w *LatencyWorkload) AverageLatencyNS(processed int64, elapsed time.Duration) float64 {
	if processed <= 0 || elapsed <= 0 {
		return 0
	}
	return float64(elapsed.Nanoseconds()) / float64(processed)
}
