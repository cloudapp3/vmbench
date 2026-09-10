package score

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
)

// maxBaselineBytes caps the size of a single baseline document.
const maxBaselineBytes = 1 << 20

//go:embed baselines.json
var embeddedBaselines []byte

var (
	embeddedOnce sync.Once
	embeddedSet  *BaselineSet
	embeddedErr  error
)

// BaselineSource names where a baseline set was loaded from.
const (
	SourceEmbedded = "embedded"
	SourcePath     = "path"
)

// EmbeddedBaseline decodes the baseline set embedded in the binary. The
// document is parsed once and the decoded set is shared between callers, so
// callers must treat the returned value as read-only.
func EmbeddedBaseline() (*BaselineSet, error) {
	embeddedOnce.Do(func() {
		embeddedSet, embeddedErr = DecodeBaseline(embeddedBaselines)
	})
	return embeddedSet, embeddedErr
}

// LoadBaseline reads and decodes a baseline set from a JSON file.
func LoadBaseline(path string) (*BaselineSet, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("score: reading baseline: %w", err)
	}
	return DecodeBaseline(data)
}

// DecodeBaseline strictly decodes one baseline document: unknown fields,
// trailing JSON documents, and oversized inputs are rejected.
func DecodeBaseline(data []byte) (*BaselineSet, error) {
	if len(data) > maxBaselineBytes {
		return nil, fmt.Errorf("score: baseline document exceeds %d bytes", maxBaselineBytes)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	set := &BaselineSet{}
	if err := decoder.Decode(set); err != nil {
		return nil, fmt.Errorf("score: invalid baseline document: %w", err)
	}
	if decoder.More() {
		return nil, errors.New("score: baseline document contains trailing data")
	}
	if err := set.Validate(); err != nil {
		return nil, err
	}
	return set, nil
}
