package float

import (
	"context"
	"errors"
	"math"
	"math/cmplx"
	"time"

	"github.com/cloudapp3/vmbench/bench"
	"github.com/cloudapp3/vmbench/bench/common"
)

const defaultFFTSize = 1 << 20

// FFTWorkload benchmarks an iterative radix-2 FFT over complex128 samples.
type FFTWorkload struct {
	input       []complex128
	inputEnergy float64
	lastEnergy  float64
	lastDigest  uint64
}

// NewFFTWorkload returns the default FFT workload.
func NewFFTWorkload() bench.Workload {
	return newFFTWorkload(defaultFFTSize)
}

func newFFTWorkload(size int) *FFTWorkload {
	input := make([]complex128, size)
	energy := 0.0
	for idx := range input {
		realPart := math.Sin(float64(idx)*0.013) + math.Cos(float64(idx)*0.021)
		imagPart := math.Sin(float64(idx)*0.037) - math.Cos(float64(idx)*0.005)
		value := complex(realPart, imagPart)
		input[idx] = value
		energy += cmplx.Abs(value) * cmplx.Abs(value)
	}
	return &FFTWorkload{input: input, inputEnergy: energy}
}

func (w *FFTWorkload) Name() string { return "FFT" }

func (w *FFTWorkload) Category() string { return bench.CategoryFloat }

func (w *FFTWorkload) Description() string {
	return "Runs an iterative radix-2 FFT over 2^20 complex128 samples."
}

func (*FFTWorkload) ProcessedKind() bench.ProcessedKind { return bench.ProcessedOperations }

func (w *FFTWorkload) Clone() bench.Workload {
	cp := *w
	cp.lastEnergy = 0
	cp.lastDigest = 0
	return &cp
}

func (w *FFTWorkload) Run(ctx context.Context) (time.Duration, int64, error) {
	select {
	case <-ctx.Done():
		return 0, 0, ctx.Err()
	default:
	}
	values := append([]complex128(nil), w.input...)
	started := time.Now()
	fft(values)
	if ctx.Err() != nil {
		return 0, 0, ctx.Err()
	}
	energy := 0.0
	var digest uint64
	stride := len(values) / 128
	if stride == 0 {
		stride = 1
	}
	for idx, value := range values {
		mag := cmplx.Abs(value)
		energy += mag * mag
		if idx%stride == 0 {
			digest ^= uint64(real(value)*1e6) + uint64(imag(value)*1e6) + uint64(idx+1)
		}
	}
	w.lastEnergy = energy / float64(len(values))
	w.lastDigest = digest
	common.ConsumeUint64(digest)
	return time.Since(started), int64(len(values)), nil
}

func (w *FFTWorkload) Validate() error {
	if w.lastDigest == 0 {
		return errors.New("fft digest is zero")
	}
	if w.inputEnergy == 0 || math.Abs(w.lastEnergy-w.inputEnergy) > w.inputEnergy*1e-6 {
		return errors.New("fft energy mismatch")
	}
	return nil
}

func (w *FFTWorkload) Throughput(processed int64, elapsed time.Duration) (float64, string) {
	if processed <= 0 || elapsed <= 0 {
		return 0, "samples/s"
	}
	return float64(processed) / elapsed.Seconds(), "samples/s"
}

func fft(values []complex128) {
	n := len(values)
	j := 0
	for i := 1; i < n; i++ {
		bit := n >> 1
		for ; j&bit != 0; bit >>= 1 {
			j &^= bit
		}
		j |= bit
		if i < j {
			values[i], values[j] = values[j], values[i]
		}
	}
	for size := 2; size <= n; size <<= 1 {
		half := size >> 1
		theta := -2 * math.Pi / float64(size)
		phaseStep := complex(math.Cos(theta), math.Sin(theta))
		for start := 0; start < n; start += size {
			phase := complex(1.0, 0)
			for idx := 0; idx < half; idx++ {
				even := values[start+idx]
				odd := phase * values[start+idx+half]
				values[start+idx] = even + odd
				values[start+idx+half] = even - odd
				phase *= phaseStep
			}
		}
	}
}
