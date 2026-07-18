package common

import (
	"math/rand"
	"strings"
)

// Seed is the fixed random seed used for reproducible benchmark inputs.
const Seed int64 = 42

// NewRand returns a deterministic random source.
func NewRand() *rand.Rand {
	return rand.New(rand.NewSource(Seed))
}

// RandomBytes returns a deterministic byte slice of the requested size.
func RandomBytes(size int) []byte {
	out := make([]byte, size)
	rng := NewRand()
	if _, err := rng.Read(out); err != nil {
		for idx := range out {
			out[idx] = byte(rng.Intn(256))
		}
	}
	return out
}

// TextCorpus returns a deterministic text-like corpus with repeated patterns.
func TextCorpus(size int) []byte {
	line := "INFO user=demo action=compress path=/api/v1/items status=200 latency=12ms payload=alpha-beta-gamma-delta\n"
	if size <= 0 {
		return nil
	}
	var builder strings.Builder
	builder.Grow(size)
	for builder.Len() < size {
		builder.WriteString(line)
		builder.WriteString(line)
		builder.WriteString(strings.ToUpper(line))
	}
	text := builder.String()
	return []byte(text[:size])
}

// RandomInt64Slice returns a deterministic int64 slice.
func RandomInt64Slice(count int) []int64 {
	out := make([]int64, count)
	rng := NewRand()
	for idx := range out {
		out[idx] = rng.Int63()
	}
	return out
}
