package nodecatalog

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// UpdateOptions defines a signed catalog update. There is intentionally no
// built-in public key: callers must explicitly choose the trust root.
type UpdateOptions struct {
	ManifestURL  string
	SignatureURL string
	Signature    []byte
	PublicKey    ed25519.PublicKey
	Destination  string
	Client       *http.Client
}

// Update downloads, verifies, validates, and atomically caches a manifest.
func Update(ctx context.Context, options UpdateOptions) (Loaded, error) {
	if strings.TrimSpace(options.ManifestURL) == "" {
		return Loaded{}, fmt.Errorf("manifest URL is required")
	}
	if len(options.PublicKey) != ed25519.PublicKeySize {
		return Loaded{}, fmt.Errorf("explicit Ed25519 public key is required")
	}
	destination := strings.TrimSpace(options.Destination)
	if destination == "" {
		var err error
		destination, err = DefaultCachePath()
		if err != nil {
			return Loaded{}, err
		}
	}

	client := options.Client
	if client == nil {
		client = http.DefaultClient
	}
	manifestData, err := fetch(ctx, client, options.ManifestURL, maxManifestBytes)
	if err != nil {
		return Loaded{}, fmt.Errorf("download node catalog: %w", err)
	}
	signatureData := options.Signature
	if len(signatureData) == 0 {
		if strings.TrimSpace(options.SignatureURL) == "" {
			return Loaded{}, fmt.Errorf("detached signature is required")
		}
		signatureData, err = fetch(ctx, client, options.SignatureURL, 16<<10)
		if err != nil {
			return Loaded{}, fmt.Errorf("download node catalog signature: %w", err)
		}
	}
	if err := Verify(manifestData, signatureData, options.PublicKey); err != nil {
		return Loaded{}, err
	}
	manifest, err := Decode(manifestData)
	if err != nil {
		return Loaded{}, fmt.Errorf("signed node catalog is invalid: %w", err)
	}
	if err := atomicWrite(destination, manifestData, 0o600); err != nil {
		return Loaded{}, fmt.Errorf("cache node catalog: %w", err)
	}
	return Loaded{
		Manifest: manifest,
		Source:   SourceAuto,
		Path:     destination,
		Warning:  expirationWarning(manifest, time.Now()),
		Raw:      append([]byte(nil), manifestData...),
	}, nil
}

// Verify validates a detached Ed25519 signature over the exact manifest bytes.
func Verify(document, encodedSignature []byte, publicKey ed25519.PublicKey) error {
	if len(publicKey) != ed25519.PublicKeySize {
		return fmt.Errorf("invalid Ed25519 public key length %d", len(publicKey))
	}
	signature, err := ParseSignature(encodedSignature)
	if err != nil {
		return err
	}
	if !ed25519.Verify(publicKey, document, signature) {
		return fmt.Errorf("node catalog signature verification failed")
	}
	return nil
}

// ParsePublicKey accepts raw, base64, hex, or PEM encoded Ed25519 keys.
func ParsePublicKey(data []byte) (ed25519.PublicKey, error) {
	if len(data) == ed25519.PublicKeySize {
		return append(ed25519.PublicKey(nil), data...), nil
	}
	trimmed := bytes.TrimSpace(data)
	if block, _ := pem.Decode(trimmed); block != nil {
		key, err := x509.ParsePKIXPublicKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("parse Ed25519 public key PEM: %w", err)
		}
		publicKey, ok := key.(ed25519.PublicKey)
		if !ok {
			return nil, fmt.Errorf("public key PEM is not Ed25519")
		}
		return append(ed25519.PublicKey(nil), publicKey...), nil
	}
	decoded, err := decodeFixed(trimmed, ed25519.PublicKeySize)
	if err != nil {
		return nil, fmt.Errorf("parse Ed25519 public key: %w", err)
	}
	return ed25519.PublicKey(decoded), nil
}

// ParseSignature accepts raw, base64, or hex detached signatures.
func ParseSignature(data []byte) ([]byte, error) {
	if len(data) == ed25519.SignatureSize {
		return append([]byte(nil), data...), nil
	}
	decoded, err := decodeFixed(bytes.TrimSpace(data), ed25519.SignatureSize)
	if err != nil {
		return nil, fmt.Errorf("parse Ed25519 signature: %w", err)
	}
	return decoded, nil
}

func fetch(ctx context.Context, client *http.Client, rawURL string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("response exceeds %d bytes", limit)
	}
	return data, nil
}

func decodeFixed(data []byte, size int) ([]byte, error) {
	if len(data) == size {
		return append([]byte(nil), data...), nil
	}
	text := string(data)
	decoders := []func(string) ([]byte, error){
		base64.StdEncoding.DecodeString,
		base64.RawStdEncoding.DecodeString,
		base64.URLEncoding.DecodeString,
		base64.RawURLEncoding.DecodeString,
		hex.DecodeString,
	}
	for _, decode := range decoders {
		decoded, err := decode(text)
		if err == nil && len(decoded) == size {
			return decoded, nil
		}
	}
	return nil, fmt.Errorf("encoded value is not %d bytes", size)
}

func atomicWrite(path string, data []byte, mode os.FileMode) (returnErr error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".nodes-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		_ = tmp.Close()
		if returnErr != nil {
			_ = os.Remove(tmpName)
		}
	}()
	if err := tmp.Chmod(mode); err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	if dirHandle, err := os.Open(dir); err == nil {
		_ = dirHandle.Sync()
		_ = dirHandle.Close()
	}
	return nil
}

// ReadPublicKey loads an explicit trust root from a local file.
func ReadPublicKey(path string) (ed25519.PublicKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read public key: %w", err)
	}
	return ParsePublicKey(data)
}

// ReadSignature loads a detached signature from a local file.
func ReadSignature(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read signature: %w", err)
	}
	if len(data) == 0 {
		return nil, errors.New("signature file is empty")
	}
	return data, nil
}
