package float

import (
	"context"
	"errors"
	"time"

	"github.com/cloudapp3/vmbench/bench"
	"github.com/cloudapp3/vmbench/bench/common"
)

const (
	defaultMandelbrotWidth  = 4096
	defaultMandelbrotHeight = 4096
	defaultMandelbrotIter   = 1000
)

// MandelbrotWorkload benchmarks Mandelbrot set iteration counts over a large grid.
type MandelbrotWorkload struct {
	width      int
	height     int
	maxIter    int
	samplePts  [][2]int
	sampleVals []int
	lastDigest uint64
}

// NewMandelbrotWorkload returns the default Mandelbrot workload.
func NewMandelbrotWorkload() bench.Workload {
	return newMandelbrotWorkload(defaultMandelbrotWidth, defaultMandelbrotHeight, defaultMandelbrotIter)
}

func newMandelbrotWorkload(width, height, maxIter int) *MandelbrotWorkload {
	return &MandelbrotWorkload{
		width:      width,
		height:     height,
		maxIter:    maxIter,
		samplePts:  [][2]int{{0, 0}, {width / 2, height / 2}, {width - 1, height - 1}},
		sampleVals: make([]int, 3),
	}
}

func (w *MandelbrotWorkload) Name() string { return "Mandelbrot" }

func (w *MandelbrotWorkload) Category() string { return bench.CategoryFloat }

func (w *MandelbrotWorkload) Description() string {
	return "Renders a 4096x4096 Mandelbrot iteration field with up to 1000 iterations per point."
}

func (*MandelbrotWorkload) ProcessedKind() bench.ProcessedKind { return bench.ProcessedOperations }

func (w *MandelbrotWorkload) Clone() bench.Workload {
	cp := *w
	cp.sampleVals = make([]int, len(w.sampleVals))
	return &cp
}

func (w *MandelbrotWorkload) Run(ctx context.Context) (time.Duration, int64, error) {
	started := time.Now()
	var digest uint64
	for y := 0; y < w.height; y++ {
		if y%8 == 0 {
			select {
			case <-ctx.Done():
				return 0, 0, ctx.Err()
			default:
			}
		}
		imaginary := (float64(y)/float64(w.height))*3.0 - 1.5
		for x := 0; x < w.width; x++ {
			real := (float64(x)/float64(w.width))*3.5 - 2.5
			iterations := mandelbrotIterations(real, imaginary, w.maxIter)
			digest ^= uint64(iterations + x + y)
			for idx, point := range w.samplePts {
				if x == point[0] && y == point[1] {
					w.sampleVals[idx] = iterations
				}
			}
		}
	}
	w.lastDigest = digest
	common.ConsumeUint64(digest)
	return time.Since(started), int64(w.width * w.height), nil
}

func (w *MandelbrotWorkload) Validate() error {
	if w.lastDigest == 0 {
		return errors.New("mandelbrot digest is zero")
	}
	for idx, point := range w.samplePts {
		real := (float64(point[0])/float64(w.width))*3.5 - 2.5
		imaginary := (float64(point[1])/float64(w.height))*3.0 - 1.5
		expected := mandelbrotIterations(real, imaginary, w.maxIter)
		if expected != w.sampleVals[idx] {
			return errors.New("mandelbrot sample mismatch")
		}
	}
	return nil
}

func (w *MandelbrotWorkload) Throughput(processed int64, elapsed time.Duration) (float64, string) {
	if processed <= 0 || elapsed <= 0 {
		return 0, "pixels/s"
	}
	return float64(processed) / elapsed.Seconds(), "pixels/s"
}

func mandelbrotIterations(real, imaginary float64, maxIter int) int {
	zr, zi := 0.0, 0.0
	for iter := 0; iter < maxIter; iter++ {
		zr2 := zr*zr - zi*zi + real
		zi2 := 2*zr*zi + imaginary
		zr, zi = zr2, zi2
		if zr*zr+zi*zi > 4 {
			return iter
		}
	}
	return maxIter
}
