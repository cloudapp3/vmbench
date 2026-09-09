// Package toolbin provisions pinned static benchmark tool binaries (fio,
// sysbench) from the vmbench "tools" GitHub release. Every download is
// verified against a SHA-256 pin compiled into this package, so the trust
// root is TLS plus source review — independent of the per-release
// checksums.txt used by install.sh and selfupdate. The main release archives
// stay free of third-party binaries; fetching is strictly opt-in.
package toolbin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// DefaultBase is the download base of the stable "tools" release tag. The
// assets it points at are replaced only together with a pin update in this
// package.
const DefaultBase = "https://github.com/cloudapp3/vmbench/releases/download/tools"

// maxAssetBytes caps a single tool download.
const maxAssetBytes = 64 << 20

// Tool describes one provisionable external benchmark tool.
type Tool struct {
	// Name is the executable name used for PATH lookup ("fio", "sysbench").
	Name string
	// Version is the upstream version the pinned builds were cut from.
	Version string
	// License names the upstream license the binary is redistributed under.
	License string
	// Source points at the exact upstream source the build used; see
	// docs/THIRD-PARTY.md for the written source offer.
	Source string
	// Sums maps "goos/goarch" to the pinned SHA-256 of the release asset.
	Sums map[string]string
}

// Registry lists the tools vmbench can provision. Adding or repinning an
// entry requires rebuilding the assets (scripts/build-tools.sh), uploading
// them to the "tools" release, and updating the hashes here.
var Registry = []Tool{
	{
		Name:    "fio",
		Version: "3.39",
		License: "GPL-2.0",
		Source:  "https://github.com/axboe/fio/tree/fio-3.39",
		Sums: map[string]string{
			"linux/amd64": "ec2d6ac78fdcb639d2e9945e4380339bf138083d23776bc9d5809c3bd8f2674c",
			"linux/arm64": "16dcf34a57744f7e4c759892421a99249907fa486e6ed2731a9f9f70828f75d4",
		},
	},
	{
		Name:    "sysbench",
		Version: "1.0.20",
		License: "GPL-2.0",
		Source:  "https://github.com/akopytov/sysbench/tree/1.0.20",
		Sums: map[string]string{
			"linux/amd64": "adc1d8fd8fd4ad8f54c18b1252b13db82ef958d629ca70cf68518100bb5d375e",
			"linux/arm64": "3bc0634f902708a7e9aa4566fb14a7b9203c3e4f07f3e10730e552c66a54d910",
		},
	},
}

// Find returns the registered tool with the given executable name.
func Find(name string) (Tool, bool) {
	for _, tool := range Registry {
		if tool.Name == name {
			return tool, true
		}
	}
	return Tool{}, false
}

// AssetName returns the release asset file name for the tool build.
func AssetName(tool Tool, goos, goarch string) string {
	return fmt.Sprintf("vmbench-tools-%s-%s-%s", tool.Name, goos, goarch)
}

// ArchSuffix maps a Go architecture to the adjacent-binary file suffix
// ("amd64" -> "x64"). It matches catalog's localToolCandidates naming so
// fetched binaries are found by the standard tool resolution.
func ArchSuffix(goarch string) string {
	switch goarch {
	case "amd64":
		return "x64"
	case "arm64":
		return "arm64"
	default:
		return ""
	}
}

// CacheDir returns the default fetch destination:
// os.UserCacheDir()/vmbench/binaries.
func CacheDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "vmbench", "binaries"), nil
}

// Options configures Fetch. Zero fields take platform defaults.
type Options struct {
	// Base is the asset URL base; it defaults to DefaultBase and may point
	// at a mirror serving the same asset names.
	Base string
	// Dest is the destination directory; it defaults to CacheDir().
	Dest string
	// GOOS and GOARCH default to runtime values; overridable for tests.
	GOOS, GOARCH string
	// Client defaults to http.DefaultClient.
	Client *http.Client
	// Version stamps the User-Agent (vmbench.Version at the call site).
	Version string
}

// Result reports a completed fetch.
type Result struct {
	Tool    Tool
	Path    string
	Bytes   int64
	Version string
}

// Fetch downloads the pinned build of tool, verifies it against the pinned
// SHA-256, and atomically installs it as <dest>/<name>_<archSuffix> with
// mode 0755. A checksum mismatch fails closed: nothing is installed.
func Fetch(ctx context.Context, tool Tool, options Options) (Result, error) {
	goos, goarch := options.GOOS, options.GOARCH
	if goos == "" {
		goos = runtime.GOOS
	}
	if goarch == "" {
		goarch = runtime.GOARCH
	}
	suffix := ArchSuffix(goarch)
	if suffix == "" {
		return Result{}, fmt.Errorf("%s: unsupported architecture %s", tool.Name, goarch)
	}
	wantSum := ""
	if tool.Sums != nil {
		wantSum = strings.ToLower(tool.Sums[goos+"/"+goarch])
	}
	if wantSum == "" || strings.Trim(wantSum, "0") == "" {
		// An all-zero pin is a placeholder that must never pass verification.
		return Result{}, fmt.Errorf("%s: no pinned build for %s/%s", tool.Name, goos, goarch)
	}

	base := strings.TrimSuffix(strings.TrimSpace(options.Base), "/")
	if base == "" {
		base = DefaultBase
	}
	dest := strings.TrimSpace(options.Dest)
	if dest == "" {
		cache, err := CacheDir()
		if err != nil {
			return Result{}, fmt.Errorf("%s: locate cache directory: %w", tool.Name, err)
		}
		dest = cache
	}

	asset := AssetName(tool, goos, goarch)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/"+asset, nil)
	if err != nil {
		return Result{}, err
	}
	version := strings.TrimSpace(options.Version)
	if version == "" {
		version = "dev"
	}
	req.Header.Set("User-Agent", "vmbench/"+version)
	client := options.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("download %s: %w", asset, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
		return Result{}, fmt.Errorf("download %s: HTTP %d", asset, resp.StatusCode)
	}

	if err := os.MkdirAll(dest, 0o755); err != nil {
		return Result{}, fmt.Errorf("%s: create destination: %w", tool.Name, err)
	}
	tmp, err := os.CreateTemp(dest, ".vmbench-tools-*.tmp")
	if err != nil {
		return Result{}, fmt.Errorf("%s: stage download: %w", tool.Name, err)
	}
	tmpName := tmp.Name()
	hash := sha256.New()
	written, err := io.Copy(io.MultiWriter(tmp, hash), io.LimitReader(resp.Body, maxAssetBytes+1))
	closeErr := tmp.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(tmpName)
		return Result{}, fmt.Errorf("download %s: %w", asset, err)
	}
	if written > maxAssetBytes {
		_ = os.Remove(tmpName)
		return Result{}, fmt.Errorf("download %s: exceeds %d bytes", asset, maxAssetBytes)
	}
	gotSum := hex.EncodeToString(hash.Sum(nil))
	if gotSum != wantSum {
		_ = os.Remove(tmpName)
		return Result{}, fmt.Errorf("checksum mismatch for %s: got %s, want %s", asset, gotSum, wantSum)
	}
	if err := os.Chmod(tmpName, 0o755); err != nil {
		_ = os.Remove(tmpName)
		return Result{}, fmt.Errorf("%s: set executable mode: %w", tool.Name, err)
	}
	target := filepath.Join(dest, tool.Name+"_"+suffix)
	if err := os.Rename(tmpName, target); err != nil {
		_ = os.Remove(tmpName)
		return Result{}, fmt.Errorf("%s: install %s: %w", tool.Name, target, err)
	}
	if dirHandle, err := os.Open(dest); err == nil {
		_ = dirHandle.Sync()
		_ = dirHandle.Close()
	}
	return Result{Tool: tool, Path: target, Bytes: written, Version: tool.Version}, nil
}
