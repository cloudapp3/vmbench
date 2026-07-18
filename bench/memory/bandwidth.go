package memory

import (
	"context"
	"errors"
	"time"

	"github.com/cloudapp3/vmbench/bench"
	"github.com/cloudapp3/vmbench/bench/common"
)

const defaultBandwidthBytes = 512 << 20

// BandwidthWorkload benchmarks sequential and random memory traffic.
type BandwidthWorkload struct {
	size         int
	source       []byte
	randomIndex  []int
	lastChecksum uint64
}

// NewBandwidthWorkload returns the default memory bandwidth workload.
func NewBandwidthWorkload() bench.Workload {
	return newBandwidthWorkload(defaultBandwidthBytes)
}

func newBandwidthWorkload(size int) *BandwidthWorkload {
	source := common.RandomBytes(size)
	step := 64
	indexCount := size / step
	indices := make([]int, indexCount)
	for idx := 0; idx < indexCount; idx++ {
		indices[idx] = idx * step
	}
	rng := common.NewRand()
	for i := len(indices) - 1; i > 0; i-- {
		j := rng.Intn(i + 1)
		indices[i], indices[j] = indices[j], indices[i]
	}
	return &BandwidthWorkload{size: size, source: source, randomIndex: indices}
}

func (w *BandwidthWorkload) Name() string { return "Mem Bandwidth" }

func (w *BandwidthWorkload) Category() string { return bench.CategoryMemory }

func (w *BandwidthWorkload) Description() string {
	return "Measures sequential copy plus randomized 64-byte memory traffic across 512 MiB buffers."
}

func (*BandwidthWorkload) ProcessedKind() bench.ProcessedKind { return bench.ProcessedBytes }

func (w *BandwidthWorkload) Clone() bench.Workload {
	cp := *w
	cp.lastChecksum = 0
	return &cp
}

func (w *BandwidthWorkload) Run(ctx context.Context) (time.Duration, int64, error) {
	dst := make([]byte, len(w.source))
	started := time.Now()
	copied := copy(dst, w.source)
	if copied != len(w.source) {
		return 0, 0, errors.New("short copy in bandwidth workload")
	}
	var checksum uint64
	for idx, offset := range w.randomIndex {
		if idx%8192 == 0 {
			select {
			case <-ctx.Done():
				return 0, 0, ctx.Err()
			default:
			}
		}
		dst[offset] ^= byte(idx)
		checksum += uint64(dst[offset]) + uint64(w.source[offset])
	}
	for idx, offset := range w.randomIndex {
		if idx%8192 == 0 {
			select {
			case <-ctx.Done():
				return 0, 0, ctx.Err()
			default:
			}
		}
		checksum ^= uint64(dst[offset])
	}
	w.lastChecksum = checksum
	common.ConsumeUint64(checksum)
	processed := int64(len(w.source)*2 + len(w.randomIndex)*2)
	return time.Since(started), processed, nil
}

func (w *BandwidthWorkload) Validate() error {
	if w.lastChecksum == 0 {
		return errors.New("bandwidth checksum is zero")
	}
	return nil
}
