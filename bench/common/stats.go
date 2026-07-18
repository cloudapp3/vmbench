package common

import (
	"sort"
	"time"
)

// MedianDuration returns the median sample.
func MedianDuration(samples []time.Duration) time.Duration {
	if len(samples) == 0 {
		return 0
	}
	cp := append([]time.Duration(nil), samples...)
	sort.Slice(cp, func(i, j int) bool { return cp[i] < cp[j] })
	return cp[len(cp)/2]
}

// MedianInt64 returns the median int64 sample.
func MedianInt64(samples []int64) int64 {
	if len(samples) == 0 {
		return 0
	}
	cp := append([]int64(nil), samples...)
	sort.Slice(cp, func(i, j int) bool { return cp[i] < cp[j] })
	return cp[len(cp)/2]
}

// MedianFloat64 returns the median float64 sample.
func MedianFloat64(samples []float64) float64 {
	if len(samples) == 0 {
		return 0
	}
	cp := append([]float64(nil), samples...)
	sort.Float64s(cp)
	return cp[len(cp)/2]
}

// MBPerSecond calculates MiB/s throughput.
func MBPerSecond(processed int64, elapsed time.Duration) float64 {
	if processed <= 0 || elapsed <= 0 {
		return 0
	}
	return (float64(processed) / (1024 * 1024)) / elapsed.Seconds()
}
