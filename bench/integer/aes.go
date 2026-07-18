package integer

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"errors"
	"time"

	"github.com/cloudapp3/vmbench/bench"
	"github.com/cloudapp3/vmbench/bench/common"
)

const defaultAESBytes = 256 << 20

// AESWorkload benchmarks AES-256-GCM encrypt/decrypt throughput.
type AESWorkload struct {
	plaintext    []byte
	nonce        []byte
	aad          []byte
	key          [32]byte
	expectedHash [32]byte
	lastHash     [32]byte
}

// NewAESWorkload returns the default AES-256-GCM benchmark workload.
func NewAESWorkload() bench.Workload {
	return newAESWorkload(defaultAESBytes)
}

func newAESWorkload(size int) *AESWorkload {
	payload := common.RandomBytes(size)
	workload := &AESWorkload{
		plaintext: payload,
		nonce:     []byte("0123456789ab"),
		aad:       []byte("gobench-aad"),
	}
	copy(workload.key[:], []byte("0123456789abcdef0123456789abcdef"))
	workload.expectedHash = sha256.Sum256(payload)
	return workload
}

func (w *AESWorkload) Name() string { return "AES-256-GCM" }

func (w *AESWorkload) Category() string { return bench.CategoryInteger }

func (w *AESWorkload) Description() string {
	return "Encrypts and decrypts 256 MiB using AES-256-GCM and validates the plaintext digest."
}

func (*AESWorkload) ProcessedKind() bench.ProcessedKind { return bench.ProcessedBytes }

func (w *AESWorkload) Clone() bench.Workload {
	cp := *w
	return &cp
}

func (w *AESWorkload) Run(ctx context.Context) (time.Duration, int64, error) {
	select {
	case <-ctx.Done():
		return 0, 0, ctx.Err()
	default:
	}
	block, err := aes.NewCipher(w.key[:])
	if err != nil {
		return 0, 0, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return 0, 0, err
	}
	started := time.Now()
	ciphertext := gcm.Seal(nil, w.nonce[:gcm.NonceSize()], w.plaintext, w.aad)
	plaintext, err := gcm.Open(nil, w.nonce[:gcm.NonceSize()], ciphertext, w.aad)
	if err != nil {
		return 0, 0, err
	}
	w.lastHash = sha256.Sum256(plaintext)
	common.ConsumeBytes(ciphertext)
	common.ConsumeBytes(plaintext[:256])
	return time.Since(started), int64(len(w.plaintext)), nil
}

func (w *AESWorkload) Validate() error {
	if w.lastHash != w.expectedHash {
		return errors.New("aes validation failed")
	}
	return nil
}
