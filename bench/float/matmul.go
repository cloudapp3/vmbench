package float

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/cloudapp3/vmbench/bench"
	"github.com/cloudapp3/vmbench/bench/common"
)

const defaultMatMulSize = 1024

// MatMulWorkload benchmarks blocked matrix multiplication on float64 matrices.
type MatMulWorkload struct {
	size        int
	blockSize   int
	a           []float64
	b           []float64
	sampleCells [][2]int
	samples     []float64
	lastDigest  uint64
}

// NewMatMulWorkload returns the default matrix multiplication workload.
func NewMatMulWorkload() bench.Workload {
	return newMatMulWorkload(defaultMatMulSize, 32)
}

func newMatMulWorkload(size, blockSize int) *MatMulWorkload {
	a := make([]float64, size*size)
	b := make([]float64, size*size)
	for idx := range a {
		a[idx] = math.Sin(float64(idx)*0.001) + 1
		b[idx] = math.Cos(float64(idx)*0.002) + 1
	}
	return &MatMulWorkload{
		size:        size,
		blockSize:   blockSize,
		a:           a,
		b:           b,
		sampleCells: [][2]int{{0, 0}, {size / 2, size / 2}, {size - 1, size - 1}},
		samples:     make([]float64, 3),
	}
}

func (w *MatMulWorkload) Name() string { return "MatMul" }

func (w *MatMulWorkload) Category() string { return bench.CategoryFloat }

func (w *MatMulWorkload) Description() string {
	return "Multiplies two 1024x1024 float64 matrices using a blocked DGEMM-style kernel."
}

func (*MatMulWorkload) ProcessedKind() bench.ProcessedKind { return bench.ProcessedOperations }

func (w *MatMulWorkload) Clone() bench.Workload {
	cp := *w
	cp.samples = make([]float64, len(w.samples))
	return &cp
}

func (w *MatMulWorkload) Run(ctx context.Context) (time.Duration, int64, error) {
	select {
	case <-ctx.Done():
		return 0, 0, ctx.Err()
	default:
	}
	n := w.size
	c := make([]float64, n*n)
	started := time.Now()
	for ii := 0; ii < n; ii += w.blockSize {
		for kk := 0; kk < n; kk += w.blockSize {
			for jj := 0; jj < n; jj += w.blockSize {
				imax := minInt(ii+w.blockSize, n)
				kmax := minInt(kk+w.blockSize, n)
				jmax := minInt(jj+w.blockSize, n)
				for i := ii; i < imax; i++ {
					if i%32 == 0 {
						select {
						case <-ctx.Done():
							return 0, 0, ctx.Err()
						default:
						}
					}
					row := i * n
					for k := kk; k < kmax; k++ {
						aval := w.a[row+k]
						krow := k * n
						for j := jj; j < jmax; j++ {
							c[row+j] += aval * w.b[krow+j]
						}
					}
				}
			}
		}
	}
	w.lastDigest = 0
	for idx, cell := range w.sampleCells {
		value := c[cell[0]*n+cell[1]]
		w.samples[idx] = value
		w.lastDigest ^= uint64(value * 1e6)
	}
	common.ConsumeUint64(w.lastDigest)
	ops := int64(2) * int64(n) * int64(n) * int64(n)
	return time.Since(started), ops, nil
}

func (w *MatMulWorkload) Validate() error {
	if w.lastDigest == 0 {
		return errors.New("matmul digest is zero")
	}
	for idx, cell := range w.sampleCells {
		expected := dotRowCol(w.a, w.b, w.size, cell[0], cell[1])
		if math.Abs(expected-w.samples[idx]) > 1e-6*math.Max(1, math.Abs(expected)) {
			return errors.New("matmul sample mismatch")
		}
	}
	return nil
}

func (w *MatMulWorkload) Throughput(processed int64, elapsed time.Duration) (float64, string) {
	if processed <= 0 || elapsed <= 0 {
		return 0, "GFLOP/s"
	}
	return (float64(processed) / 1e9) / elapsed.Seconds(), "GFLOP/s"
}

func dotRowCol(a, b []float64, n, row, col int) float64 {
	sum := 0.0
	rowOffset := row * n
	for k := 0; k < n; k++ {
		sum += a[rowOffset+k] * b[k*n+col]
	}
	return sum
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
