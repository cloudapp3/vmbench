package integer

import (
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"errors"
	"hash"
	"time"

	"github.com/cloudapp3/vmbench/bench"
	"github.com/cloudapp3/vmbench/bench/common"
)

const defaultHashBytes = 256 << 20

// HashWorkload benchmarks repeated hashing over a fixed byte slice.
type HashWorkload struct {
	name           string
	description    string
	data           []byte
	newHash        func() hash.Hash
	expectedDigest []byte
	lastDigest     []byte
}

// NewSHA256Workload returns the default SHA-256 workload.
func NewSHA256Workload() bench.Workload {
	return newHashWorkload("SHA-256", "Hashes 256 MiB using SHA-256.", sha256.New, defaultHashBytes)
}

// NewSHA512Workload returns the default SHA-512 workload.
func NewSHA512Workload() bench.Workload {
	return newHashWorkload("SHA-512", "Hashes 256 MiB using SHA-512.", sha512.New, defaultHashBytes)
}

func newHashWorkload(name, description string, factory func() hash.Hash, size int) *HashWorkload {
	data := common.RandomBytes(size)
	h := factory()
	_, _ = h.Write(data)
	return &HashWorkload{
		name:           name,
		description:    description,
		data:           data,
		newHash:        factory,
		expectedDigest: h.Sum(nil),
	}
}

func (w *HashWorkload) Name() string { return w.name }

func (w *HashWorkload) Category() string { return bench.CategoryInteger }

func (w *HashWorkload) Description() string { return w.description }

func (*HashWorkload) ProcessedKind() bench.ProcessedKind { return bench.ProcessedBytes }

func (w *HashWorkload) Clone() bench.Workload {
	cp := *w
	cp.lastDigest = nil
	return &cp
}

func (w *HashWorkload) Run(ctx context.Context) (time.Duration, int64, error) {
	select {
	case <-ctx.Done():
		return 0, 0, ctx.Err()
	default:
	}
	h := w.newHash()
	started := time.Now()
	_, err := h.Write(w.data)
	if err != nil {
		return 0, 0, err
	}
	w.lastDigest = h.Sum(nil)
	common.ConsumeBytes(w.lastDigest)
	return time.Since(started), int64(len(w.data)), nil
}

func (w *HashWorkload) Validate() error {
	if len(w.lastDigest) == 0 || string(w.lastDigest) != string(w.expectedDigest) {
		return errors.New("hash validation failed")
	}
	return nil
}
