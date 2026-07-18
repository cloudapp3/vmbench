package common

import "sync/atomic"

var sink atomic.Uint64

// ConsumeUint64 mixes a value into the global sink.
func ConsumeUint64(value uint64) {
	sink.Add(value + 0x9e3779b97f4a7c15)
}

// ConsumeBytes mixes a byte slice into the global sink.
func ConsumeBytes(data []byte) {
	var mixed uint64
	for _, value := range data {
		mixed = mixed*131 + uint64(value)
	}
	ConsumeUint64(mixed)
}

// SinkValue returns the current sink value.
func SinkValue() uint64 {
	return sink.Load()
}
