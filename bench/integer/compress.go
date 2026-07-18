package integer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/cloudapp3/vmbench/bench"
	"github.com/cloudapp3/vmbench/bench/common"
	"github.com/klauspost/compress/zstd"
	"github.com/pierrec/lz4/v4"
)

const defaultCompressBytes = 100 << 20

// CompressWorkload benchmarks compression plus decompression validation.
type CompressWorkload struct {
	name           string
	description    string
	codec          string
	data           []byte
	expectedDigest [32]byte
	lastDigest     [32]byte
}

// NewLZ4Workload returns the default LZ4 compression workload.
func NewLZ4Workload() bench.Workload {
	return newCompressWorkload("LZ4 Compress", "LZ4 compresses and decompresses 100 MiB of text-like data.", "lz4", defaultCompressBytes)
}

// NewZstdWorkload returns the default Zstd compression workload.
func NewZstdWorkload() bench.Workload {
	return newCompressWorkload("Zstd Compress", "Zstd level 3 compresses and decompresses 100 MiB of text-like data.", "zstd", defaultCompressBytes)
}

func newCompressWorkload(name, description, codec string, size int) *CompressWorkload {
	data := common.TextCorpus(size)
	return &CompressWorkload{
		name:           name,
		description:    description,
		codec:          codec,
		data:           data,
		expectedDigest: sha256.Sum256(data),
	}
}

func (w *CompressWorkload) Name() string { return w.name }

func (w *CompressWorkload) Category() string { return bench.CategoryInteger }

func (w *CompressWorkload) Description() string { return w.description }

func (*CompressWorkload) ProcessedKind() bench.ProcessedKind { return bench.ProcessedBytes }

func (w *CompressWorkload) Clone() bench.Workload {
	cp := *w
	return &cp
}

func (w *CompressWorkload) Run(ctx context.Context) (time.Duration, int64, error) {
	select {
	case <-ctx.Done():
		return 0, 0, ctx.Err()
	default:
	}
	started := time.Now()
	var decoded []byte
	switch w.codec {
	case "lz4":
		var buf bytes.Buffer
		writer := lz4.NewWriter(&buf)
		if _, err := writer.Write(w.data); err != nil {
			return 0, 0, err
		}
		if err := writer.Close(); err != nil {
			return 0, 0, err
		}
		reader := lz4.NewReader(bytes.NewReader(buf.Bytes()))
		payload, err := io.ReadAll(reader)
		if err != nil {
			return 0, 0, err
		}
		decoded = payload
		common.ConsumeBytes(buf.Bytes()[:min(len(buf.Bytes()), 256)])
	case "zstd":
		encoder, err := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.EncoderLevelFromZstd(3)))
		if err != nil {
			return 0, 0, err
		}
		encoded := encoder.EncodeAll(w.data, nil)
		if err := encoder.Close(); err != nil {
			return 0, 0, err
		}
		decoder, err := zstd.NewReader(nil)
		if err != nil {
			return 0, 0, err
		}
		payload, err := decoder.DecodeAll(encoded, nil)
		decoder.Close()
		if err != nil {
			return 0, 0, err
		}
		decoded = payload
		common.ConsumeBytes(encoded[:min(len(encoded), 256)])
	default:
		return 0, 0, fmt.Errorf("unknown codec: %s", w.codec)
	}
	w.lastDigest = sha256.Sum256(decoded)
	return time.Since(started), int64(len(w.data)), nil
}

func (w *CompressWorkload) Validate() error {
	if w.lastDigest != w.expectedDigest {
		return errors.New("compression validation failed")
	}
	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
