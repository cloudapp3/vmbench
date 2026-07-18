package diskio

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	"github.com/cloudapp3/vmbench/bench"
	"github.com/cloudapp3/vmbench/bench/common"
)

const (
	defaultSequentialBytes = 256 << 20
	defaultRandomBytes     = 64 << 20
	randomBlockSize        = 4 << 10
	randomOperations       = 10_000
)

// SequentialWorkload benchmarks sequential disk write and read throughput.
type SequentialWorkload struct {
	root         string
	size         int
	payload      []byte
	lastChecksum uint64
}

// RandomWorkload benchmarks 4K random disk IOPS.
type RandomWorkload struct {
	root         string
	size         int
	payload      []byte
	lastChecksum uint64
}

// NewSequentialWorkload returns the default sequential disk workload.
func NewSequentialWorkload(root string) bench.Workload {
	return &SequentialWorkload{root: root, size: defaultSequentialBytes, payload: common.RandomBytes(defaultSequentialBytes)}
}

// NewRandomWorkload returns the default random IOPS disk workload.
func NewRandomWorkload(root string) bench.Workload {
	return &RandomWorkload{root: root, size: defaultRandomBytes, payload: common.RandomBytes(defaultRandomBytes)}
}

func (w *SequentialWorkload) Name() string     { return "Disk Sequential" }
func (w *SequentialWorkload) Category() string { return bench.CategoryExtensionDisk }
func (w *SequentialWorkload) Description() string {
	return "Writes and reads a 256 MiB temporary file to measure sequential throughput."
}
func (*SequentialWorkload) ProcessedKind() bench.ProcessedKind { return bench.ProcessedBytes }
func (w *SequentialWorkload) Clone() bench.Workload            { cp := *w; cp.lastChecksum = 0; return &cp }

func (w *SequentialWorkload) Run(ctx context.Context) (time.Duration, int64, error) {
	root := chooseRoot(w.root)
	file, err := os.CreateTemp(root, "gobench-seq-*.bin")
	if err != nil {
		return 0, 0, err
	}
	name := file.Name()
	defer os.Remove(name)
	started := time.Now()
	if _, err := file.Write(w.payload); err != nil {
		file.Close()
		return 0, 0, err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return 0, 0, err
	}
	if _, err := file.Seek(0, 0); err != nil {
		file.Close()
		return 0, 0, err
	}
	buf := make([]byte, 1<<20)
	var checksum uint64
	totalRead := 0
	for {
		select {
		case <-ctx.Done():
			file.Close()
			return 0, 0, ctx.Err()
		default:
		}
		n, err := file.Read(buf)
		if n > 0 {
			totalRead += n
			end := min(n, 256)
			for _, value := range buf[:end] {
				checksum += uint64(value)
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			file.Close()
			return 0, 0, err
		}
		if n == 0 {
			break
		}
	}
	if totalRead != len(w.payload) {
		file.Close()
		return 0, 0, fmt.Errorf("short read in sequential workload: read %d of %d bytes", totalRead, len(w.payload))
	}
	if err := file.Close(); err != nil {
		return 0, 0, err
	}
	w.lastChecksum = checksum
	common.ConsumeUint64(checksum)
	return time.Since(started), int64(w.size * 2), nil
}

func (w *SequentialWorkload) Validate() error {
	if w.lastChecksum == 0 {
		return errors.New("disk sequential checksum is zero")
	}
	return nil
}

func (w *RandomWorkload) Name() string     { return "Disk Random 4K" }
func (w *RandomWorkload) Category() string { return bench.CategoryExtensionDisk }
func (w *RandomWorkload) Description() string {
	return "Performs 10k randomized 4K reads/writes against a 64 MiB temporary file."
}
func (*RandomWorkload) ProcessedKind() bench.ProcessedKind { return bench.ProcessedBytes }
func (w *RandomWorkload) Clone() bench.Workload            { cp := *w; cp.lastChecksum = 0; return &cp }

func (w *RandomWorkload) Run(ctx context.Context) (time.Duration, int64, error) {
	root := chooseRoot(w.root)
	name := filepath.Join(root, fmt.Sprintf("gobench-rand-%d.bin", time.Now().UnixNano()))
	if err := os.WriteFile(name, w.payload, 0o600); err != nil {
		return 0, 0, err
	}
	defer os.Remove(name)
	file, err := os.OpenFile(name, os.O_RDWR, 0o600)
	if err != nil {
		return 0, 0, err
	}
	defer file.Close()
	rng := rand.New(rand.NewSource(common.Seed))
	block := make([]byte, randomBlockSize)
	started := time.Now()
	var checksum uint64
	for op := 0; op < randomOperations; op++ {
		if op%256 == 0 {
			select {
			case <-ctx.Done():
				return 0, 0, ctx.Err()
			default:
			}
		}
		offset := int64(rng.Intn((w.size/randomBlockSize)-1) * randomBlockSize)
		if _, err := file.ReadAt(block, offset); err != nil {
			return 0, 0, err
		}
		checksum += uint64(block[0])
		block[0] ^= byte(op)
		if _, err := file.WriteAt(block, offset); err != nil {
			return 0, 0, err
		}
	}
	w.lastChecksum = checksum
	common.ConsumeUint64(checksum)
	return time.Since(started), int64(randomOperations * randomBlockSize * 2), nil
}

func (w *RandomWorkload) Validate() error {
	if w.lastChecksum == 0 {
		return errors.New("disk random checksum is zero")
	}
	return nil
}

func (w *RandomWorkload) Throughput(processed int64, elapsed time.Duration) (float64, string) {
	if elapsed <= 0 {
		return 0, "IOPS"
	}
	return float64(randomOperations) / elapsed.Seconds(), "IOPS"
}

func chooseRoot(root string) string {
	if root == "" {
		return os.TempDir()
	}
	return root
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
