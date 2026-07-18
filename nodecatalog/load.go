package nodecatalog

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const maxManifestBytes = 4 << 20

//go:embed nodes.json
var embeddedManifest []byte

var (
	embeddedOnce  sync.Once
	embeddedValue Manifest
	embeddedErr   error
)

// LoadOptions controls selection of an embedded, cached, or explicit catalog.
type LoadOptions struct {
	Source    string
	Revision  string
	CachePath string
}

// Loaded contains the resolved manifest plus provenance needed by reports.
type Loaded struct {
	Manifest Manifest `json:"catalog"`
	Source   string   `json:"source"`
	Path     string   `json:"path,omitempty"`
	Warning  string   `json:"warning,omitempty"`
	Raw      []byte   `json:"-"`
}

// Embedded returns an isolated copy of the built-in offline snapshot.
func Embedded() (Manifest, error) {
	embeddedOnce.Do(func() {
		embeddedValue, embeddedErr = Decode(embeddedManifest)
	})
	if embeddedErr != nil {
		return Manifest{}, embeddedErr
	}
	return embeddedValue.Clone(), nil
}

// Load resolves embedded, auto, or an explicit path and enforces a revision pin.
func Load(options LoadOptions) (Loaded, error) {
	source := strings.TrimSpace(options.Source)
	if source == "" {
		source = SourceEmbedded
	}
	pin := strings.TrimSpace(options.Revision)

	switch strings.ToLower(source) {
	case SourceEmbedded:
		return loadEmbedded(pin)
	case SourceAuto:
		return loadAuto(options.CachePath, pin)
	default:
		loaded, err := loadPath(source)
		if err != nil {
			return Loaded{}, err
		}
		if err := checkRevision(loaded.Manifest, pin); err != nil {
			return Loaded{}, err
		}
		loaded.Warning = expirationWarning(loaded.Manifest, time.Now())
		return loaded, nil
	}
}

// Decode parses exactly one strict JSON document and validates its contract.
func Decode(data []byte) (Manifest, error) {
	if len(data) == 0 {
		return Manifest{}, fmt.Errorf("node catalog is empty")
	}
	if len(data) > maxManifestBytes {
		return Manifest{}, fmt.Errorf("node catalog exceeds %d bytes", maxManifestBytes)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var manifest Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode node catalog: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return Manifest{}, fmt.Errorf("decode node catalog: trailing JSON value")
		}
		return Manifest{}, fmt.Errorf("decode node catalog: %w", err)
	}
	if err := manifest.Validate(); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

// DefaultCachePath returns the per-user destination used by auto loading.
func DefaultCachePath() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("resolve user cache directory: %w", err)
	}
	return filepath.Join(dir, "vmbench", "nodes.json"), nil
}

func loadEmbedded(pin string) (Loaded, error) {
	manifest, err := Embedded()
	if err != nil {
		return Loaded{}, fmt.Errorf("load embedded node catalog: %w", err)
	}
	if err := checkRevision(manifest, pin); err != nil {
		return Loaded{}, err
	}
	return Loaded{
		Manifest: manifest,
		Source:   SourceEmbedded,
		Warning:  expirationWarning(manifest, time.Now()),
		Raw:      append([]byte(nil), embeddedManifest...),
	}, nil
}

func loadAuto(cachePath, pin string) (Loaded, error) {
	path := strings.TrimSpace(cachePath)
	if path == "" {
		var err error
		path, err = DefaultCachePath()
		if err != nil {
			embedded, embeddedErr := loadEmbedded(pin)
			if embeddedErr != nil {
				return Loaded{}, errors.Join(err, embeddedErr)
			}
			embedded.Warning = err.Error()
			return embedded, nil
		}
	}

	cached, cacheErr := loadPath(path)
	if cacheErr == nil {
		if revisionErr := checkRevision(cached.Manifest, pin); revisionErr == nil {
			cached.Source = SourceAuto
			cached.Warning = expirationWarning(cached.Manifest, time.Now())
			return cached, nil
		} else {
			cacheErr = revisionErr
		}
	}

	embedded, embeddedErr := loadEmbedded(pin)
	if embeddedErr != nil {
		return Loaded{}, fmt.Errorf("auto node catalog: cached: %v; embedded: %w", cacheErr, embeddedErr)
	}
	if !errors.Is(cacheErr, os.ErrNotExist) {
		embedded.Warning = fmt.Sprintf("cached catalog ignored: %v", cacheErr)
	}
	return embedded, nil
}

func loadPath(path string) (Loaded, error) {
	file, err := os.Open(path)
	if err != nil {
		return Loaded{}, fmt.Errorf("open node catalog %s: %w", path, err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxManifestBytes+1))
	if err != nil {
		return Loaded{}, fmt.Errorf("read node catalog %s: %w", path, err)
	}
	manifest, err := Decode(data)
	if err != nil {
		return Loaded{}, fmt.Errorf("load node catalog %s: %w", path, err)
	}
	return Loaded{Manifest: manifest, Source: SourcePath, Path: path, Raw: data}, nil
}

func checkRevision(manifest Manifest, pin string) error {
	if pin != "" && manifest.Revision != pin {
		return fmt.Errorf("node catalog revision %q does not match pinned revision %q", manifest.Revision, pin)
	}
	return nil
}

func expirationWarning(manifest Manifest, now time.Time) string {
	if !manifest.ExpiresAt.After(now) {
		return fmt.Sprintf("node catalog revision %s expired at %s", manifest.Revision, manifest.ExpiresAt.UTC().Format(time.RFC3339))
	}
	return ""
}
