package integer

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/cloudapp3/vmbench/bench"
	"github.com/cloudapp3/vmbench/bench/common"
)

const defaultSortCount = 10_000_000

// SortWorkload benchmarks sorting a large deterministic int64 slice.
type SortWorkload struct {
	input         []int64
	expectedCheck uint64
	lastCheck     uint64
	lastSorted    bool
}

// NewSortWorkload returns the default sorting workload.
func NewSortWorkload() bench.Workload {
	return newSortWorkload(defaultSortCount)
}

func newSortWorkload(count int) *SortWorkload {
	data := common.RandomInt64Slice(count)
	cp := append([]int64(nil), data...)
	slices.Sort(cp)
	return &SortWorkload{input: data, expectedCheck: checksumInt64(cp)}
}

func (w *SortWorkload) Name() string { return "Sort" }

func (w *SortWorkload) Category() string { return bench.CategoryInteger }

func (w *SortWorkload) Description() string {
	return "Sorts 10 million int64 values using slices.Sort and validates the sorted checksum."
}

func (*SortWorkload) ProcessedKind() bench.ProcessedKind { return bench.ProcessedBytes }

func (w *SortWorkload) Clone() bench.Workload {
	cp := *w
	cp.lastCheck = 0
	cp.lastSorted = false
	return &cp
}

func (w *SortWorkload) Run(ctx context.Context) (time.Duration, int64, error) {
	select {
	case <-ctx.Done():
		return 0, 0, ctx.Err()
	default:
	}
	data := append([]int64(nil), w.input...)
	started := time.Now()
	slices.Sort(data)
	w.lastSorted = slices.IsSorted(data)
	w.lastCheck = checksumInt64(data)
	common.ConsumeUint64(w.lastCheck)
	return time.Since(started), int64(len(data) * 8), nil
}

func (w *SortWorkload) Validate() error {
	if !w.lastSorted || w.lastCheck != w.expectedCheck {
		return errors.New("sort validation failed")
	}
	return nil
}

func checksumInt64(values []int64) uint64 {
	var sum uint64
	for idx, value := range values {
		sum ^= uint64(value) + uint64(idx+1)*0x9e3779b97f4a7c15
	}
	return sum
}
